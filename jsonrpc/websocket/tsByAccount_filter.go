package websocket

import (
	"sync"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/jsonrpc/args"
)

// tesseractByAccountFilter is a filter to log the updates of tesseract based on address
type tesseractByAccountFilter struct {
	filterBase
	sync.Mutex

	address       identifiers.Address
	rpcTesseracts []*args.RPCTesseract
}

func (f *tesseractByAccountFilter) getSubscriptionType() subscriptionType {
	return NewTesseractsByAccount
}

func (f *tesseractByAccountFilter) appendTesseracts(rpcTesseract *args.RPCTesseract) {
	f.Lock()
	defer f.Unlock()

	f.rpcTesseracts = append(f.rpcTesseracts, rpcTesseract)
}

func (f *tesseractByAccountFilter) takeTesseractUpdates() []*args.RPCTesseract {
	f.Lock()
	defer f.Unlock()

	rpcTesseracts := f.rpcTesseracts
	// create brand-new slice to prevent new tesseracts from being added to current tesseracts
	f.rpcTesseracts = []*args.RPCTesseract{}

	return rpcTesseracts
}

func (f *tesseractByAccountFilter) getUpdates() (interface{}, error) {
	rpcTesseracts := f.takeTesseractUpdates()

	return rpcTesseracts, nil
}

// sendUpdate writes the new tesseracts to web socket stream, if the filter ID and tesseract address matches
func (f *tesseractByAccountFilter) sendUpdates() error {
	rpcTesseracts := f.takeTesseractUpdates()

	if rpcTesseracts != nil {
		return sendTesseract(rpcTesseracts, &f.filterBase)
	}

	return nil
}
