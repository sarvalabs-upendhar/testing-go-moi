package forage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pkg/errors"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
)

const (
	maxAccMetaInfoEntriesPerMsg = 50
)

type LatestAccountInfo struct {
	Address         identifiers.Address
	Height          uint64
	Hash            common.Hash
	IsSnapAvailable bool
}

// SYNCRPCService is a struct that represents an SYNC RPC Service
type SYNCRPCService struct {
	syncer              *Syncer
	snapProcessingCh    chan struct{}
	latticeProcessingCh chan struct{}
}

func NewSyncRPCService(syncer *Syncer) *SYNCRPCService {
	return &SYNCRPCService{
		syncer:              syncer,
		snapProcessingCh:    make(chan struct{}, 1),
		latticeProcessingCh: make(chan struct{}, 3),
	}
}

func (service *SYNCRPCService) SyncSnap(
	ctx context.Context,
	req <-chan *SnapRequest,
	resp chan<- common.SnapResponse,
) error {
	defer func() {
		close(resp)
	}()

	select {
	case service.snapProcessingCh <- struct{}{}:
	default:
		service.syncer.logger.Trace("another snap sync request being handled")

		return errors.New("another snap sync request being handled")
	}

	defer func() {
		<-service.snapProcessingCh
	}()

	var (
		snapReq *SnapRequest
		ok      bool
	)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case snapReq, ok = <-req:
		if !ok {
			log.Println("request channel closed")

			return nil
		}
	}

	sentSnapSize, err := service.syncer.db.StreamSnapshot(ctx, snapReq.Address, 0, resp)
	if err != nil {
		service.syncer.logger.Error("Failed to fetch account snap shot", "addr", snapReq.Address,
			"error", err)

		return err
	}

	// signal the end of whole snap
	select {
	case resp <- common.SnapResponse{
		MetaInfo: &common.SnapMetaInfo{
			CreatedAt:     time.Now().UnixNano(),
			TotalSnapSize: sentSnapSize,
		},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (service *SYNCRPCService) SyncBucketsSince(
	ctx context.Context,
	reqChan <-chan *BucketSyncRequest,
	respChan chan<- BucketSyncResponse,
) error {
	defer close(respChan)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case req, ok := <-reqChan:
			if !ok {
				return nil
			}

			bucketCount, err := service.syncer.db.GetBucketCount(req.BucketID)
			if err != nil {
				return err
			}

			if bucketCount == 0 {
				respChan <- BucketSyncResponse{
					BucketID:         req.BucketID,
					BucketCount:      0,
					AccountMetaInfos: nil,
				}

				continue
			}

			rawData, err := service.syncer.db.GetRecentUpdatedAccMetaInfosRaw(ctx, req.BucketID, req.Timestamp)
			if err != nil {
				return errors.Wrap(err, "failed to load meta infos")
			}

			if len(rawData) == 0 {
				respChan <- BucketSyncResponse{
					BucketID:         req.BucketID,
					BucketCount:      0,
					AccountMetaInfos: nil,
				}

				continue
			}

			currentPosition := uint64(0)

			pendingCount := uint64(len(rawData))
			for pendingCount > 0 {
				resp := BucketSyncResponse{
					BucketID:         req.BucketID,
					BucketCount:      uint64(len(rawData)),
					AccountMetaInfos: make([][]byte, 0, maxAccMetaInfoEntriesPerMsg),
				}

				if pendingCount <= maxAccMetaInfoEntriesPerMsg {
					resp.AccountMetaInfos = rawData
				} else {
					resp.AccountMetaInfos = rawData[currentPosition : currentPosition+maxAccMetaInfoEntriesPerMsg]
					currentPosition += maxAccMetaInfoEntriesPerMsg
				}

				respChan <- resp

				pendingCount -= uint64(len(resp.AccountMetaInfos))
			}
		}
	}
}

func (service *SYNCRPCService) GetLatestAccountInfo(
	ctx context.Context,
	addr identifiers.Address,
	accountStatus *LatestAccountInfo,
) error {
	metaInfo, err := service.syncer.db.GetAccountMetaInfo(addr)
	if err != nil {
		service.syncer.logger.Error(
			"Failed to fetch account meta information",
			"addr", addr,
			"err", err,
		)

		return errors.New("failed to fetch account info")
	}

	accountStatus.Height = metaInfo.Height
	accountStatus.Address = metaInfo.Address
	accountStatus.Hash = metaInfo.TesseractHash
	accountStatus.IsSnapAvailable = true // TODO: Improve this, all nodes maynot support snapshot

	return nil
}

func (service *SYNCRPCService) SyncLattice(
	ctx context.Context,
	reqChan <-chan *LatticeRequest,
	respChan chan<- *networkmsg.TesseractSyncMsg,
) error {
	var (
		req *LatticeRequest
		ok  bool
	)

	select {
	case service.latticeProcessingCh <- struct{}{}:
	default:
		service.syncer.logger.Trace("Too many requests in progress for lattice sync. Request rejected.")

		close(respChan)

		return errors.New("Too many requests in progress for lattice sync. Request rejected.")
	}

	defer func() {
		<-service.latticeProcessingCh
	}()

	select {
	case <-ctx.Done():
		close(respChan)

		return ctx.Err()
	case req, ok = <-reqChan:
		if !ok {
			log.Println("request channel closed")

			return nil
		}
	}

	defer func() {
		close(respChan)
	}()

	for height := req.StartHeight; height <= req.EndHeight; height++ {
		ts, ixns, receipts, err := service.syncer.getTesseractWithRawIxnsAndReceipts(
			req.Address,
			height,
			true,
			true,
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to fetch tesseract %d", height))
		}

		msg := &networkmsg.TesseractSyncMsg{
			RawTesseract: make([]byte, 0),
			Ixns:         ixns,
			Receipts:     receipts,
			Delta:        make(map[string][]byte),
		}

		msg.RawTesseract, err = ts.Canonical().Bytes()
		if err != nil {
			return err
		}

		if ts.ICSHash() != common.NilHash {
			icsClusterInfoRaw, err := service.syncer.db.ReadEntry(ts.ICSHash().Bytes())
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to fetch ics cluster info %v for address %v ",
					ts.ICSHash(), req.Address))
			}

			msg.Delta[ts.ICSHash().String()] = icsClusterInfoRaw
		}

		for addr, contextHash := range ts.PreviousContext() {
			if contextHash.IsNil() {
				continue
			}

			if err = service.syncer.state.GetParticipantContextRaw(addr, contextHash, msg.Delta); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to fetch participant context for %v", addr))
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case respChan <- msg:
		}
	}

	return nil
}

func (service *SYNCRPCService) SyncBuckets(
	ctx context.Context,
	reqChan <-chan *BucketSyncRequest,
	respChan chan<- BucketSyncResponse,
) error {
	var (
		req *BucketSyncRequest
		ok  bool
	)

	defer close(respChan)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req, ok = <-reqChan:
			if !ok {
				return nil
			}
		}

		dbResponse := make(chan []byte)

		count, err := service.syncer.db.GetBucketCount(req.BucketID)
		if err != nil {
			return errors.Wrap(err, "failed to fetch bucket count")
		}

		if count == 0 {
			respChan <- BucketSyncResponse{
				BucketID:         req.BucketID,
				BucketCount:      count,
				AccountMetaInfos: nil,
			}

			continue
		}

		go func() {
			if err = service.syncer.db.StreamAccountMetaInfosRaw(ctx, req.BucketID, dbResponse); err != nil {
				service.syncer.logger.Error(
					"Failed to stream account meta information from DB",
					"err", err,
				)
			}
		}()

		resp := BucketSyncResponse{
			BucketID:         req.BucketID,
			BucketCount:      count,
			AccountMetaInfos: make([][]byte, 0, maxAccMetaInfoEntriesPerMsg),
		}
		pendingCount := count

		for rawData := range dbResponse {
			resp.AccountMetaInfos = append(resp.AccountMetaInfos, rawData)

			if len(resp.AccountMetaInfos) == maxAccMetaInfoEntriesPerMsg || uint64(len(resp.AccountMetaInfos)) == count {
				respChan <- resp

				pendingCount -= uint64(len(resp.AccountMetaInfos))

				resp = BucketSyncResponse{
					BucketID:         req.BucketID,
					BucketCount:      count,
					AccountMetaInfos: make([][]byte, 0, 50),
				}
			}

			if pendingCount == 0 {
				break
			}
		}
	}
}
