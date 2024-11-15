package consensus

import (
	"context"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/state"
)

func (k *Engine) GetICSCommittee(
	ts *common.Tesseract,
	info *common.CommitInfo,
) (*types.ICSCommittee, error) {
	addrs := ts.Addresses()
	ps := ts.Participants()

	ics := types.NewICSCommittee(len(addrs) + 1)

	for index, addr := range addrs {
		if ps[addr].PreviousContext == common.NilHash {
			continue
		}

		behaviourSet, _, err := k.fetchParticipantContextByHash(addr, ps[addr].PreviousContext)
		if err != nil {
			return nil, err
		}

		if behaviourSet != nil {
			ics.UpdateNodeSet(index, behaviourSet)
		}
	}

	randomSet, err := k.NodeSet(info.RandomSet, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.StochasticSetPosition(), randomSet)

	return ics, nil
}

func (k *Engine) NodeSet(ids []kramaid.KramaID, setSizeWithoutDelta uint32) (*types.NodeSet, error) {
	var (
		publicKeys [][]byte
		err        error
	)

	if len(ids) == 0 {
		return nil, err
	}

	publicKeys, err = k.state.GetPublicKeys(context.Background(), ids...)
	if err != nil {
		return nil, err
	}

	return types.NewNodeSet(ids, publicKeys, setSizeWithoutDelta), nil
}

func (k *Engine) NodeSetFromRawContextObject(raw []byte) (*types.NodeSet, error) {
	obj := new(state.ContextObject)
	if err := obj.FromBytes(raw); err != nil {
		return nil, err
	}

	return k.NodeSet(obj.Ids, uint32(len(obj.Ids)))
}

func (k *Engine) GetICSCommitteeFromRawContext(
	ts *common.Tesseract,
	rawContext map[string][]byte,
	info *common.CommitInfo,
) (*types.ICSCommittee, error) {
	contextHashes := make([]common.Hash, 0)
	addrs := ts.Addresses()
	ps := ts.Participants()

	ics := types.NewICSCommittee(len(addrs) + 1)

	for index, addr := range addrs {
		position := index

		if ps[addr].PreviousContext == common.NilHash {
			continue
		}

		metaObject := new(state.MetaContextObject)
		if err := metaObject.FromBytes(rawContext[ps[addr].PreviousContext.String()]); err != nil {
			return nil, err
		}

		rawBytes, ok := rawContext[metaObject.BehaviouralContext.String()]
		if ok {
			nodeSet, err := k.NodeSetFromRawContextObject(rawBytes)
			if err != nil {
				return nil, err
			}

			ics.UpdateNodeSet(position, nodeSet)
		}

		contextHashes = append(contextHashes, ps[addr].PreviousContext)
		contextHashes = append(contextHashes, metaObject.BehaviouralContext)
		contextHashes = append(contextHashes, metaObject.RandomContext)
	}

	// delete the context hashes from delta separately instead of deleting in above for loop
	// because sender and receiver context nodes can be same then we cannot extract receiver context nodes
	for _, hash := range contextHashes {
		// delete context hashes, inorder to avoid writing dirty entries to db
		delete(rawContext, hash.String())
	}

	randomSet, err := k.NodeSet(info.RandomSet, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.UpdateNodeSet(ics.StochasticSetPosition(), randomSet)

	return ics, nil
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (k *Engine) fetchParticipantContextByHash(addr identifiers.Address, hash common.Hash) (
	behaviouralSet, randomSet *types.NodeSet,
	err error,
) {
	behaviouralContext, randomContext, err := k.state.GetContext(addr, hash)
	if err != nil {
		k.logger.Error("failed to retrieve context nodes", "err", err, "addr", addr)

		return nil, nil, err
	}

	if len(behaviouralContext) > 0 {
		publicKeys, err := k.state.GetPublicKeys(context.Background(), behaviouralContext...)
		if err != nil {
			k.logger.Error("failed to retrieve the public key of behavioural set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}

		behaviouralSet = types.NewNodeSet(behaviouralContext, publicKeys, uint32(len(behaviouralContext)))
	}

	if len(randomContext) > 0 {
		publicKeys, err := k.state.GetPublicKeys(context.Background(), randomContext...)
		if err != nil {
			k.logger.Error("failed to retrieve the public key of random set", "err", err)

			return nil, nil, common.ErrPublicKeyNotFound
		}

		randomSet = types.NewNodeSet(randomContext, publicKeys, uint32(len(randomContext)))
	}

	return behaviouralSet, randomSet, nil
}
