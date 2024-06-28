package jsonrpc

import (
	"sync"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
)

type logFilter struct {
	filterBase
	sync.RWMutex
	query *LogQuery
	logs  []*args.RPCLog
}

func (f *logFilter) getSubscriptionType() subscriptionType {
	return NewLogsByFilter
}

// appendLog appends new log to logs
func (f *logFilter) appendLog(log []*args.RPCLog) {
	f.Lock()
	defer f.Unlock()

	f.logs = append(f.logs, log...)
}

// takeLogUpdates returns all saved logs in filter and set new log slice
func (f *logFilter) takeLogUpdates() []*args.RPCLog {
	f.Lock()
	defer f.Unlock()

	logs := f.logs
	// create brand-new slice to prevent new logs from being added to current logs
	f.logs = []*args.RPCLog{}

	return logs
}

func (f *logFilter) getUpdates() (interface{}, error) {
	logs := f.takeLogUpdates()

	return logs, nil
}

// sendUpdate writes the new logs from receipt to web socket stream
func (f *logFilter) sendUpdates() error {
	logs := f.takeLogUpdates()

	if err := sendLogs(logs, &f.filterBase); err != nil {
		return err
	}

	return nil
}

// createRPCLogs filters tesseract logs based on filter topics, and returns logs to websocket
func (f *logFilter) createRPCLogs(
	ts *common.Tesseract,
	rpcTS *args.RPCTesseract,
) []*args.RPCLog {
	logs := make([]*args.RPCLog, 0)

	for _, receipt := range ts.Receipts() {
		for _, log := range receipt.Logs {
			if f.query.MatchTopics(log) {
				logs = append(logs,
					&args.RPCLog{
						Address:      log.Address,
						LogicID:      log.LogicID,
						Topics:       log.Topics,
						Data:         log.Data,
						IxHash:       receipt.IxHash,
						TSHash:       rpcTS.Hash,
						Participants: rpcTS.Participants,
					},
				)
			}
		}
	}

	return logs
}
