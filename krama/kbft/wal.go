package kbft

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"path/filepath"
	"time"

	"github.com/sarvalabs/moichain/types"

	ktypes "github.com/sarvalabs/moichain/krama/types"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
)

const (
	DefaultFlushInterval = 2 * time.Second
	maxMsgSizeBytes      = 1024 * 12
)

var crc32c = crc32.MakeTable(crc32.Castagnoli)

type WALMessage interface{}

// TimedWALMessage captures the WAL message with timestamp
type TimedWALMessage struct {
	ClusterID types.ClusterID
	Time      time.Time
	Msg       WALMessage
}

// ICSInitCheckpoint marks the start of an ICS inside WAL.
type ICSInitCheckpoint struct {
	ClusterID    string
	IxHash       string
	Participants []string
}

// WAL is an interface for any write-ahead logger.
type WAL interface {
	Write(ktypes.ConsensusMessage, types.ClusterID) error
	WriteSync(ktypes.ConsensusMessage, types.ClusterID) error
	FlushAndSync() error

	SearchForClusterID(clusterID string, options *WALSearchOptions) (rd io.ReadCloser, found bool, err error)

	Start() error
	Close()
	// Wait()
}

// Write ahead logger writes msgs to disk before they are processed.
// Can be used for crash-recovery and deterministic replay.

type BaseWAL struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    hclog.Logger

	group *utils.Group

	enc *WALEncoder

	flushTicker   *time.Ticker
	flushInterval time.Duration
}

// NewWAL returns a new write-ahead logger based on `baseWAL`, which implements
// WAL. It's flushed and synced to disk every 2s and once when stopped.
func NewWAL(
	ctx context.Context,
	logger hclog.Logger,
	walFile string,
	groupOptions ...func(*utils.Group),
) (*BaseWAL, error) {
	err := utils.EnsureDir(filepath.Dir(walFile+"/wal"), 0o700)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure WAL directory is in place: %w", err)
	}

	group, err := utils.OpenGroup(ctx, logger, walFile+"/wal", groupOptions...)
	if err != nil {
		return nil, err
	}

	ctx, cancelFn := context.WithCancel(ctx)
	wal := &BaseWAL{
		ctx:           ctx,
		ctxCancel:     cancelFn,
		logger:        logger,
		group:         group,
		enc:           NewWALEncoder(group),
		flushInterval: DefaultFlushInterval,
	}

	return wal, nil
}

// SetFlushInterval allows us to override the periodic flush interval for the WAL.
func (wal *BaseWAL) SetFlushInterval(i time.Duration) {
	wal.flushInterval = i
}

// Group returns the wal group
func (wal *BaseWAL) Group() *utils.Group {
	return wal.group
}

func (wal *BaseWAL) Start() error {
	size, err := wal.group.Head.Size()
	if err != nil {
		return err
	} else if size == 0 {
		err = wal.WriteSync(ktypes.ConsensusMessage{PeerID: "init", Message: nil}, "init")
		if err != nil {
			return err
		}
	}

	err = wal.group.Start(wal.ctx)
	if err != nil {
		return err
	}

	wal.flushTicker = time.NewTicker(wal.flushInterval)

	go wal.processFlushTicks(wal.ctx)

	return nil
}

func (wal *BaseWAL) processFlushTicks(ctx context.Context) {
	for {
		select {
		case <-wal.flushTicker.C:
			if err := wal.FlushAndSync(); err != nil {
				wal.logger.Error("Periodic WAL flush failed", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// FlushAndSync flushes and fsync's the underlying group's data to disk.
// See auto#FlushAndSync
func (wal *BaseWAL) FlushAndSync() error {
	return wal.group.FlushAndSync()
}

// Close the underlying autofile group.
func (wal *BaseWAL) Close() {
	wal.flushTicker.Stop()

	if err := wal.FlushAndSync(); err != nil {
		wal.logger.Error("error on flush data to disk", "error", err)
	}

	wal.group.Stop()
	wal.group.Close()
}

// Wait for the underlying autofile group to finish shutting down
// so it's safe to cleanup files.
/*
func (wal *BaseWAL) Wait() {
	if wal.IsRunning() {
		wal.BaseService.Wait()
	}
	if wal.group.IsRunning() {
		wal.group.Wait()
	}
}
*/

// Write is called in newStep and for each receive on the
// peerMsgQueue and the timeoutTicker.
// NOTE: does not call fsync()
func (wal *BaseWAL) Write(msg ktypes.ConsensusMessage, clusterID types.ClusterID) error {
	if wal == nil {
		return nil
	}

	walMsg, err := msg.WALMsg()
	if err != nil {
		return err
	}

	if err = wal.enc.Encode(&ktypes.TimedWALMessage{
		ClusterID: clusterID,
		Timestamp: time.Now().UnixNano(),
		Message:   walMsg,
	}); err != nil {
		wal.logger.Error(
			"error writing msg to consensus wal."+
				" WARNING: recover may not be possible for the current height",
			"err", err,
			"msg", msg,
			"msgType", walMsg.MsgType,
		)

		return err
	}

	return nil
}

// WriteSync is called when we receive a msg from ourselves
// so that we write to disk before sending signed messages.
// NOTE: calls fsync()
func (wal *BaseWAL) WriteSync(msg ktypes.ConsensusMessage, clusterID types.ClusterID) error {
	if wal == nil {
		return nil
	}

	if err := wal.Write(msg, clusterID); err != nil {
		return err
	}

	if err := wal.FlushAndSync(); err != nil {
		wal.logger.Error(`WriteSync failed to flush consensus wal.
		WARNING: may result in creating alternative proposals / votes for the current height iff the node restarted`,
			"err", err)

		return err
	}

	return nil
}

// WALSearchOptions are optional arguments to SearchForEndHeight.
type WALSearchOptions struct {
	// IgnoreDataCorruptionErrors set to true will result in skipping data corruption errors.
	IgnoreDataCorruptionErrors bool
}

// SearchForClusterID searches for the NewICSMessage with the clusterID
// and returns an auto.GroupReader, whenever it was found or not and an error.
// Group reader will be nil if found equals false.
//
// CONTRACT: caller must close group reader.
func (wal *BaseWAL) SearchForClusterID(
	clusterID string,
	options *WALSearchOptions,
) (rd io.ReadCloser, found bool, err error) {
	var (
		msg *ktypes.TimedWALMessage
		gr  *utils.GroupReader
	)

	// NOTE: starting from the last file in the group because we're usually
	// searching for the last height. See replay.go
	min, max := wal.group.MinIndex(), wal.group.MaxIndex()
	wal.logger.Info("Searching for cluster id", "cluster-id", clusterID, "min", min, "max", max)

	for index := max; index >= min; index-- {
		gr, err = wal.group.NewReader(index)
		if err != nil {
			return nil, false, err
		}

		dec := NewWALDecoder(gr)

		for {
			msg, err = dec.Decode()
			if errors.Is(err, io.EOF) {
				// // OPTIMISATION: no need to look for height in older files if we've seen h < height
				// if lastHeightFound > 0 && lastHeightFound < height {
				//	 gr.Close()
				//	 return nil, false, nil
				// }
				// check next file
				break
			}

			if options.IgnoreDataCorruptionErrors && IsDataCorruptionError(err) {
				wal.logger.Error("Corrupted entry. Skipping...", "err", err)
				// do nothing
				continue
			} else if err != nil {
				grErr := gr.Close()
				if grErr != nil {
					wal.logger.Error("Failed to close group reader", "err", grErr)
				}

				return nil, false, err
			}

			if string(msg.ClusterID) == clusterID { // found
				wal.logger.Info("Found", "cluster-id", clusterID, "index", index)

				return gr, true, nil
			}
		}

		err := gr.Close()
		if err != nil {
			wal.logger.Error("Failed to close group reader", "err", err)
		}
	}

	return nil, false, nil
}

// A WALEncoder writes custom-encoded WAL messages to an output stream.
//
// Format: 4 bytes CRC sum + 4 bytes length + arbitrary-length value
type WALEncoder struct {
	wr io.Writer
}

// NewWALEncoder returns a new encoder that writes to wr.
func NewWALEncoder(wr io.Writer) *WALEncoder {
	return &WALEncoder{wr}
}

// Encode writes the custom encoding of v to the stream. It returns an error if
// the encoded size of v is greater than 1MB. Any error encountered
// during the write is also returned.
func (enc *WALEncoder) Encode(v *ktypes.TimedWALMessage) error {
	rawData, err := v.Bytes()
	if err != nil {
		return err
	}

	crc := crc32.Checksum(rawData, crc32c)

	length := uint32(len(rawData))
	if length > maxMsgSizeBytes {
		return fmt.Errorf("msg is too big: %d bytes, max: %d bytes", length, maxMsgSizeBytes)
	}

	totalLength := 8 + int(length)
	msg := make([]byte, totalLength)

	binary.BigEndian.PutUint32(msg[0:4], crc)
	binary.BigEndian.PutUint32(msg[4:8], length)
	copy(msg[8:], rawData)

	_, err = enc.wr.Write(msg)

	return err
}

// IsDataCorruptionError returns true if data has been corrupted inside WAL.
func IsDataCorruptionError(err error) bool {
	var dataCorruptionError *DataCorruptionError

	return errors.As(err, &dataCorruptionError)
}

// DataCorruptionError is an error that occures if data on disk was corrupted.
type DataCorruptionError struct {
	cause error
}

func (e DataCorruptionError) Error() string {
	return fmt.Sprintf("DataCorruptionError[%v]", e.cause)
}

func (e DataCorruptionError) Cause() error {
	return e.cause
}

// A WALDecoder reads and decodes custom-encoded WAL messages from an input
// stream. See WALEncoder for the format used.
//
// It will also compare the checksums and make sure data size is equal to the
// length from the header. If that is not the case, error will be returned.
type WALDecoder struct {
	rd io.Reader
}

// NewWALDecoder returns a new decoder that reads from rd.
func NewWALDecoder(rd io.Reader) *WALDecoder {
	return &WALDecoder{rd}
}

// Decode reads the next custom-encoded value from its reader and returns it.
func (dec *WALDecoder) Decode() (*ktypes.TimedWALMessage, error) {
	b := make([]byte, 4)

	_, err := dec.rd.Read(b)
	if errors.Is(err, io.EOF) {
		return nil, err
	}

	if err != nil {
		return nil, DataCorruptionError{fmt.Errorf("failed to read checksum: %w", err)}
	}

	crc := binary.BigEndian.Uint32(b)
	b = make([]byte, 4)

	_, err = dec.rd.Read(b)
	if err != nil {
		return nil, DataCorruptionError{fmt.Errorf("failed to read length: %w", err)}
	}

	length := binary.BigEndian.Uint32(b)
	if length > maxMsgSizeBytes {
		return nil, DataCorruptionError{fmt.Errorf(
			"length %d exceeded maximum possible value of %d bytes",
			length,
			maxMsgSizeBytes)}
	}

	data := make([]byte, length)

	n, err := dec.rd.Read(data)
	if err != nil {
		return nil, DataCorruptionError{fmt.Errorf("failed to read data: %w (read: %d, wanted: %d)", err, n, length)}
	}

	// check checksum before decoding data
	actualCRC := crc32.Checksum(data, crc32c)
	if actualCRC != crc {
		return nil, DataCorruptionError{fmt.Errorf("checksums do not match: read: %v, actual: %v", crc, actualCRC)}
	}

	res := new(ktypes.TimedWALMessage)

	if err = res.FromBytes(data); err != nil {
		return nil, DataCorruptionError{fmt.Errorf("failed to decode data: %w", err)}
	}

	return res, err
}
