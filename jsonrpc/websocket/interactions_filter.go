package websocket

import (
	"encoding/json"
	"sync"

	"github.com/sarvalabs/go-moi/common"
)

// pendingIxnsFilter is a filter to store pending ixHashes
type pendingIxnsFilter struct {
	filterBase
	sync.Mutex

	ixHashes []common.Hash
}

func (f *pendingIxnsFilter) getSubscriptionType() subscriptionType {
	return PendingIxns
}

// appendPendingIxHashes appends new pending ixn hash to ixHashes
func (f *pendingIxnsFilter) appendPendingIxHashes(ixns common.Interactions) {
	f.Lock()
	defer f.Unlock()

	for _, ixn := range ixns {
		f.ixHashes = append(f.ixHashes, ixn.Hash())
	}
}

// takePendingTxsUpdates returns all saved pending ixHashes in filter and sets a new slice
func (f *pendingIxnsFilter) takePendingIxnsUpdates() []common.Hash {
	f.Lock()
	defer f.Unlock()

	ixHashes := f.ixHashes
	// create brand-new slice to prevent new ixHashes from being added to current ixHashes
	f.ixHashes = []common.Hash{}

	return ixHashes
}

// getUpdates returns stored pending tx hashes
func (f *pendingIxnsFilter) getUpdates() (interface{}, error) {
	pendingIxHashes := f.takePendingIxnsUpdates()

	return pendingIxHashes, nil
}

// sendUpdates write the hashes for all pending transactions to web socket stream
func (f *pendingIxnsFilter) sendUpdates() error {
	pendingIxHashes := f.takePendingIxnsUpdates()
	for _, ixHash := range pendingIxHashes {
		str, err := json.Marshal(ixHash.String())
		if err != nil {
			return err
		}

		if err = f.writeMessageToWs(string(str)); err != nil {
			return err
		}
	}

	return nil
}
