package consensus

import (
	"context"

	"github.com/sarvalabs/go-moi/common/identifiers"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/state"
)

func (k *Engine) GetICSCommittee(
	ts *common.Tesseract,
	info *common.CommitInfo,
) (*types.ICSCommittee, error) {
	ids := ts.AccountIDs()
	ps := ts.Participants()

	ics := types.NewICSCommittee(len(ids) + 1)

	for index, id := range ids {
		if ps[id].PreviousContext == common.NilHash {
			continue
		}

		consensusNodes, err := k.fetchParticipantContextByHash(id, ps[id].PreviousContext)
		if err != nil {
			return nil, err
		}

		if consensusNodes != nil {
			ics.UpdateNodeSet(index, consensusNodes)
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

func (k *Engine) GetICSCommitteeFromRawContext(
	ts *common.Tesseract,
	rawContext map[string][]byte,
	info *common.CommitInfo,
) (*types.ICSCommittee, error) {
	contextHashes := make([]common.Hash, 0)
	ids := ts.AccountIDs()
	ps := ts.Participants()

	ics := types.NewICSCommittee(len(ids) + 1)

	for index, id := range ids {
		position := index

		if ps[id].PreviousContext == common.NilHash {
			continue
		}

		metaObject := new(state.MetaContextObject)
		if err := metaObject.FromBytes(rawContext[ps[id].PreviousContext.String()]); err != nil {
			return nil, err
		}

		nodeSet, err := k.NodeSet(metaObject.ConsensusNodes, uint32(len(metaObject.ConsensusNodes)))
		if err != nil {
			return nil, err
		}

		ics.UpdateNodeSet(position, nodeSet)

		contextHashes = append(contextHashes, ps[id].PreviousContext)
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
func (k *Engine) fetchParticipantContextByHash(id identifiers.Identifier, hash common.Hash) (
	consensusSet *types.NodeSet,
	err error,
) {
	consensusNodes, err := k.state.GetConsensusNodes(id, hash)
	if err != nil {
		k.logger.Error("failed to retrieve context nodes", "err", err, "id", id)

		return nil, err
	}

	if len(consensusNodes) > 0 {
		publicKeys, err := k.state.GetPublicKeys(context.Background(), consensusNodes...)
		if err != nil {
			k.logger.Error("failed to retrieve the public key of consensus nodes", "err", err)

			return nil, common.ErrPublicKeyNotFound
		}

		consensusSet = types.NewNodeSet(consensusNodes, publicKeys, uint32(len(consensusNodes)))
	}

	return consensusSet, nil
}
