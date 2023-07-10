package syncer

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"

	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

const (
	maxMessageSize              = 1024
	maxAccMetaInfoEntriesPerMsg = 50
)

type LatestAccountInfo struct {
	Address         types.Address
	Height          uint64
	Hash            types.Hash
	IsSnapAvailable bool
}

// SYNCRPCService is a struct that represents an SYNC RPC Service
type SYNCRPCService struct {
	syncer *Syncer
}

func NewSyncRPCService(syncer *Syncer) *SYNCRPCService {
	return &SYNCRPCService{syncer}
}

func (service *SYNCRPCService) SyncSnap(
	ctx context.Context,
	req <-chan *SnapRequest,
	resp chan<- *SnapResponse,
) error {
	defer close(resp)

	for snapReq := range req {
		snap, err := service.syncer.db.GetAccountSnapshot(ctx, snapReq.Address, 0)
		if err != nil {
			service.syncer.logger.Error("Failed to fetch account snap shot", "addr", snapReq.Address)

			return err
		}

		createdAt := time.Now().UnixNano()
		noOfMessages := int(snap.Size / maxMessageSize)

		if snap.Size%maxMessageSize != 0 {
			noOfMessages++
		}

		start := 0
		for i := 0; i < noOfMessages; i++ {
			end := start + maxMessageSize
			if end > len(snap.Entries) {
				end = len(snap.Entries)
			}

			respMsg := &SnapResponse{
				Data: make([]byte, 0, maxMessageSize),
			}

			if i == 0 {
				respMsg.MetaInfo = &SnapMetaInfo{
					CreatedAt:     createdAt,
					TotalSnapSize: snap.Size,
				}
			}

			respMsg.Data = snap.Entries[start:end]

			start = end

			select {
			case resp <- respMsg:
				break
			case <-ctx.Done():
				return ctx.Err()
			}
		}
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
	addr types.Address,
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

func (service *SYNCRPCService) FetchLattice(
	ctx context.Context,
	reqChan <-chan *LatticeRequest,
	respChan chan<- *ptypes.TesseractMessage,
) error {
	var (
		req *LatticeRequest
		ok  bool
	)

	select {
	case <-ctx.Done():
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
			return errors.Wrap(err, "failed to fetch tesseract")
		}

		msg := &ptypes.TesseractMessage{
			RawTesseract: make([]byte, 0),
			Ixns:         ixns,
			Receipts:     receipts,
			Delta:        make(map[types.Hash][]byte, 0),
		}

		msg.RawTesseract, err = ts.Canonical().Bytes()
		if err != nil {
			return err
		}

		if ts.ICSHash() != types.NilHash {
			icsClusterInfoRaw, err := service.syncer.db.ReadEntry(ts.ICSHash().Bytes())
			if err != nil {
				return err
			}

			msg.Delta[ts.ICSHash()] = icsClusterInfoRaw
		}

		for addr, lockInfo := range ts.ContextLock() {
			if lockInfo.ContextHash.IsNil() {
				continue
			}

			if err = service.syncer.state.GetParticipantContextRaw(addr, lockInfo.ContextHash, msg.Delta); err != nil {
				return err
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
