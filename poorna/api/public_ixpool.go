package api

import (
	"fmt"

	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

type ContentResponse struct {
	Pending map[types.Address]map[uint64]*ptypes.InteractionArg `json:"pending"`
	Queued  map[types.Address]map[uint64]*ptypes.InteractionArg `json:"queued"`
}

type ContentFromResponse struct {
	Pending map[uint64]*ptypes.InteractionArg `json:"pending"`
	Queued  map[uint64]*ptypes.InteractionArg `json:"queued"`
}

type StatusResponse struct {
	Pending uint64 `json:"pending"`
	Queued  uint64 `json:"queued"`
}

type InspectResponse struct {
	Pending  map[string]map[string]string `json:"pending"`
	Queued   map[string]map[string]string `json:"queued"`
	WaitTime map[string]int64             `json:"wait_time"`
}

// PublicIXPoolAPI is a struct that represents a wrapper for the public IxPool APIs.
type PublicIXPoolAPI struct {
	// Represents the API backend
	ixpool IxPool
}

func NewPublicIXPoolAPI(ixpool IxPool) *PublicIXPoolAPI {
	// Create the public ixpool API wrapper and return it
	return &PublicIXPoolAPI{ixpool}
}

// Content returns the interactions present in the IxPool.
func (p *PublicIXPoolAPI) Content() (*ContentResponse, error) {
	content := &ContentResponse{
		Pending: make(map[types.Address]map[uint64]*ptypes.InteractionArg),
		Queued:  make(map[types.Address]map[uint64]*ptypes.InteractionArg),
	}
	pendingIxs, queuedIxs := p.ixpool.GetAllIxs(true)

	// update pending ixs
	for addr, ixs := range pendingIxs {
		content.Pending[addr] = make(map[uint64]*ptypes.InteractionArg, len(ixs))

		for _, ix := range ixs {
			ixArg := ptypes.NewInteractionArg(ix)
			content.Pending[addr][ix.Nonce()] = ixArg
		}
	}

	// update queued ixs
	for addr, ixs := range queuedIxs {
		content.Queued[addr] = make(map[uint64]*ptypes.InteractionArg, len(ixs))

		for _, ix := range ixs {
			ixArg := ptypes.NewInteractionArg(ix)
			content.Queued[addr][ix.Nonce()] = ixArg
		}
	}

	return content, nil
}

// ContentFrom returns the interactions present in the IxPool based on the given address.
func (p *PublicIXPoolAPI) ContentFrom(args *ptypes.IxPoolArgs) (*ContentFromResponse, error) {
	addr, err := utils.ValidateAddress(args.Address)
	if err != nil {
		return nil, err
	}

	content := &ContentFromResponse{
		Pending: make(map[uint64]*ptypes.InteractionArg),
		Queued:  make(map[uint64]*ptypes.InteractionArg),
	}
	pendingIxs, queuedIxs := p.ixpool.GetIxs(addr, true)

	// update pending ixs
	for _, ix := range pendingIxs {
		ixArg := ptypes.NewInteractionArg(ix)
		content.Pending[ix.Nonce()] = ixArg
	}

	// update queued ixs
	for _, ix := range queuedIxs {
		ixArg := ptypes.NewInteractionArg(ix)
		content.Queued[ix.Nonce()] = ixArg
	}

	return content, nil
}

// Status returns the number of pending and queued interactions in the IxPool
func (p *PublicIXPoolAPI) Status() (*StatusResponse, error) {
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

	status := &StatusResponse{
		Pending: uint64(pendingIxsCount),
		Queued:  uint64(queuedIxsCount),
	}

	return status, nil
}

// Inspect retrieves the interactions present in the IxPool and converts it into a simple, readable list for inspection.
// Additionally, it provides a list of all the accounts in IxPool along with their respective wait times.
func (p *PublicIXPoolAPI) Inspect() (*InspectResponse, error) {
	content := &InspectResponse{
		Pending:  make(map[string]map[string]string),
		Queued:   make(map[string]map[string]string),
		WaitTime: make(map[string]int64),
	}
	pendingIxs, queuedIxs := p.ixpool.GetAllIxs(true)
	accountWaitTimes := p.ixpool.GetAllAccountsWaitTime()

	// Define a formatter to flatten a transaction into a string
	format := func(ix *types.Interaction) string {
		if receiver := ix.Receiver(); !receiver.IsNil() {
			return fmt.Sprintf(
				"%s: %d wei + %d gas × %d wei",
				ix.Receiver().Hex(),
				ix.Cost(),
				ix.FuelLimit(),
				ix.FuelPrice(),
			)
		}

		return fmt.Sprintf(
			"%d wei + %d gas × %d wei",
			ix.Cost(),
			ix.FuelLimit(),
			ix.FuelPrice(),
		)
	}

	// update pending ixs
	for addr, ixs := range pendingIxs {
		content.Pending[addr.Hex()] = make(map[string]string, len(ixs))

		for _, ix := range ixs {
			content.Pending[addr.Hex()][fmt.Sprintf("%d", ix.Nonce())] = format(ix)
		}
	}

	// update queued ixs
	for addr, ixs := range queuedIxs {
		content.Queued[addr.Hex()] = make(map[string]string, len(ixs))

		for _, ix := range ixs {
			content.Queued[addr.Hex()][fmt.Sprintf("%d", ix.Nonce())] = format(ix)
		}
	}

	// update wait time
	for addr, waitTime := range accountWaitTimes {
		content.WaitTime[addr.Hex()] = waitTime
	}

	return content, nil
}

// WaitTime returns the wait time for an account in IxPool, based on the queried address.
func (p *PublicIXPoolAPI) WaitTime(args *ptypes.IxPoolArgs) (int64, error) {
	addr, err := utils.ValidateAddress(args.Address)
	if err != nil {
		return 0, err
	}

	return p.ixpool.GetAccountWaitTime(addr)
}
