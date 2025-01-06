package api

import (
	"fmt"
	"math/big"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
)

// PublicIXPoolAPI is a struct that represents a wrapper for the public IxPool APIs.
type PublicIXPoolAPI struct {
	// Represents the API backend
	ixpool backend.IxPool
}

func NewPublicIXPoolAPI(ixpool backend.IxPool) *PublicIXPoolAPI {
	// Create the public ixpool API wrapper and return it
	return &PublicIXPoolAPI{ixpool}
}

// Content returns the interactions present in the IxPool.
func (p *PublicIXPoolAPI) Content() (*rpcargs.ContentResponse, error) {
	content := &rpcargs.ContentResponse{
		Pending: make(map[identifiers.Address]map[hexutil.Uint64]*rpcargs.InteractionResponse),
		Queued:  make(map[identifiers.Address]map[hexutil.Uint64]*rpcargs.InteractionResponse),
	}
	pendingIxs, queuedIxs := p.ixpool.GetAllIxs(true)

	// update pending ixs
	for addr, ixs := range pendingIxs {
		content.Pending[addr] = make(map[hexutil.Uint64]*rpcargs.InteractionResponse, len(ixs))

		for _, ix := range ixs {
			ixArg := rpcargs.NewInteractionResponse(ix)
			content.Pending[addr][hexutil.Uint64(ix.SequenceID())] = ixArg
		}
	}

	// update queued ixs
	for addr, ixs := range queuedIxs {
		content.Queued[addr] = make(map[hexutil.Uint64]*rpcargs.InteractionResponse, len(ixs))

		for _, ix := range ixs {
			ixArg := rpcargs.NewInteractionResponse(ix)
			content.Queued[addr][hexutil.Uint64(ix.SequenceID())] = ixArg
		}
	}

	return content, nil
}

// ContentFrom returns the interactions present in the IxPool based on the given address.
func (p *PublicIXPoolAPI) ContentFrom(args *rpcargs.IxPoolArgs) (*rpcargs.ContentFromResponse, error) {
	if args.Address.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	content := &rpcargs.ContentFromResponse{
		Pending: make(map[hexutil.Uint64]*rpcargs.InteractionResponse),
		Queued:  make(map[hexutil.Uint64]*rpcargs.InteractionResponse),
	}
	pendingIxs, queuedIxs := p.ixpool.GetIxs(args.Address, true)

	// update pending ixs
	for _, ix := range pendingIxs {
		ixArg := rpcargs.NewInteractionResponse(ix)
		content.Pending[hexutil.Uint64(ix.SequenceID())] = ixArg
	}

	// update queued ixs
	for _, ix := range queuedIxs {
		ixArg := rpcargs.NewInteractionResponse(ix)
		content.Queued[hexutil.Uint64(ix.SequenceID())] = ixArg
	}

	return content, nil
}

// Status returns the number of pending and queued interactions in the IxPool
func (p *PublicIXPoolAPI) Status() (*rpcargs.StatusResponse, error) {
	var (
		pendingIxsCount int
		queuedIxsCount  int
	)

	pendingIxs, queuedIxs := p.ixpool.GetAllIxs(true)

	for _, ixs := range pendingIxs {
		pendingIxsCount += len(ixs)
	}

	for _, ixs := range queuedIxs {
		queuedIxsCount += len(ixs)
	}

	return &rpcargs.StatusResponse{
		Pending: hexutil.Uint64(pendingIxsCount),
		Queued:  hexutil.Uint64(queuedIxsCount),
	}, nil
}

// Inspect retrieves the interactions present in the IxPool and converts it into a simple, readable list for inspection.
// Additionally, it provides a list of all the accounts in IxPool along with their respective wait times.
func (p *PublicIXPoolAPI) Inspect() (*rpcargs.InspectResponse, error) {
	content := &rpcargs.InspectResponse{
		Pending:  make(map[string]map[string]string),
		Queued:   make(map[string]map[string]string),
		WaitTime: make(map[string]*rpcargs.WaitTimeResponse),
	}
	pendingIxs, queuedIxs := p.ixpool.GetAllIxs(true)
	accountWaitTimes := p.ixpool.GetAllAccountsWaitTime()

	// Define a formatter to flatten an interaction into a string
	format := func(ix *common.Interaction) string {
		return fmt.Sprintf(
			"%d kmoi + %d fuel × %d kmoi",
			ix.Cost(),
			ix.FuelLimit(),
			ix.FuelPrice(),
		)
	}

	// update pending ixs
	for addr, ixs := range pendingIxs {
		content.Pending[addr.Hex()] = make(map[string]string, len(ixs))

		for _, ix := range ixs {
			content.Pending[addr.Hex()][fmt.Sprintf("%d", ix.SequenceID())] = format(ix)
		}
	}

	// update queued ixs
	for addr, ixs := range queuedIxs {
		content.Queued[addr.Hex()] = make(map[string]string, len(ixs))

		for _, ix := range ixs {
			content.Queued[addr.Hex()][fmt.Sprintf("%d", ix.SequenceID())] = format(ix)
		}
	}

	// update wait time
	for addr, waitTime := range accountWaitTimes {
		content.WaitTime[addr.Hex()] = createWaitTime(waitTime)
	}

	return content, nil
}

// WaitTime returns the wait time for an account in IxPool, based on the queried address.
func (p *PublicIXPoolAPI) WaitTime(args *rpcargs.IxPoolArgs) (*rpcargs.WaitTimeResponse, error) {
	if args.Address.IsNil() {
		return nil, common.ErrInvalidAddress
	}

	waitTime, err := p.ixpool.GetAccountWaitTime(args.Address)
	if err != nil {
		return nil, err
	}

	return createWaitTime(waitTime), nil
}

func createWaitTime(waitTime *big.Int) *rpcargs.WaitTimeResponse {
	var expired bool

	if waitTime.Sign() <= 0 {
		expired = true
	}

	waitTime.Abs(waitTime)

	return &rpcargs.WaitTimeResponse{
		Expired: expired,
		Time:    (*hexutil.Big)(waitTime),
	}
}
