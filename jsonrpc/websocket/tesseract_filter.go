package websocket

import (
	"sync"

	"github.com/sarvalabs/go-moi/jsonrpc/args"
)

// tesseractFilter is a filter to log the updates of tesseract
type tesseractFilter struct {
	filterBase
	sync.Mutex

	rpcTesseracts []*args.RPCTesseract
}

func (f *tesseractFilter) getSubscriptionType() subscriptionType {
	return NewTesseract
}

func (f *tesseractFilter) appendTesseracts(rpcTesseract *args.RPCTesseract) {
	f.Lock()
	defer f.Unlock()

	f.rpcTesseracts = append(f.rpcTesseracts, rpcTesseract)
}

func (f *tesseractFilter) takeTesseractUpdates() []*args.RPCTesseract {
	f.Lock()
	defer f.Unlock()

	rpcTesseracts := f.rpcTesseracts
	// create brand-new slice to prevent new tesseracts from being added to current tesseracts
	f.rpcTesseracts = []*args.RPCTesseract{}

	return rpcTesseracts
}

func (f *tesseractFilter) getUpdates() (interface{}, error) {
	rpcTesseracts := f.takeTesseractUpdates()

	return rpcTesseracts, nil
}

// sendUpdate writes the new tesseracts to web socket stream
func (f *tesseractFilter) sendUpdates() error {
	rpcTesseracts := f.takeTesseractUpdates()

	if rpcTesseracts != nil {
		return sendTesseract(rpcTesseracts, &f.filterBase)
	}

	return nil
}
