package consensus

import (
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/state"
)

func (k *Engine) NodeSet(
	validators []*common.ValidatorInfo,
	setSizeWithoutDelta uint32,
) (*types.NodeSet, error) {
	if len(validators) == 0 {
		return nil, nil
	}

	return types.NewNodeSet(validators, setSizeWithoutDelta), nil
}

func (k *Engine) GetICSCommittee(
	ts *common.Tesseract,
	info *common.CommitInfo,
	systemObject *state.SystemObject,
) (*types.ICSCommittee, error) {
	ids := ts.AccountIDs()
	ps := ts.Participants()

	ics := types.NewICSCommittee()

	for _, id := range ids {
		if ps[id].LockedContext == common.NilHash {
			continue
		}

		consensusNodes, consensusNodesHash, err := k.fetchParticipantContextByHash(ics, id, ps[id].LockedContext)
		if err != nil {
			return nil, err
		}

		ics.UpdateNodeset(consensusNodesHash, consensusNodes, ps[id], ts.ConsensusInfo().AccountLocks[id])
	}

	vals, err := systemObject.GetValidators(info.RandomSet...)
	if err != nil {
		return nil, err
	}

	randomSet, err := k.NodeSet(vals, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.AppendNodeSet(common.NilHash, randomSet)

	return ics, nil
}

func (k *Engine) GetICSCommitteeFromRawContext(
	ts *common.Tesseract,
	rawContext map[string][]byte,
	info *common.CommitInfo,
	systemObject *state.SystemObject,
) (*types.ICSCommittee, error) {
	contextHashes := make([]common.Hash, 0)
	ids := ts.AccountIDs()
	ps := ts.Participants()

	ics := types.NewICSCommittee()

	for _, id := range ids {
		if ps[id].LockedContext == common.NilHash {
			continue
		}

		metaObject := new(state.MetaContextObject)
		if err := metaObject.FromBytes(rawContext[ps[id].LockedContext.String()]); err != nil {
			return nil, err
		}

		validators, err := systemObject.GetValidatorsByKramaID(metaObject.ConsensusNodes)
		if err != nil {
			return nil, err
		}

		nodeSet, err := k.NodeSet(validators, uint32(len(metaObject.ConsensusNodes)))
		if err != nil {
			return nil, err
		}

		contextHashes = append(contextHashes, ps[id].LockedContext)

		ics.UpdateNodeset(metaObject.ConsensusNodesHash, nodeSet, ps[id], ts.ConsensusInfo().AccountLocks[id])
	}

	// delete the context hashes from delta separately instead of deleting in above for loop
	// because sender and receiver context nodes can be same then we cannot extract receiver context nodes
	for _, hash := range contextHashes {
		// delete context hashes, inorder to avoid writing dirty entries to db
		delete(rawContext, hash.String())
	}

	validators, err := systemObject.GetValidators(info.RandomSet...)
	if err != nil {
		return nil, err
	}

	randomSet, err := k.NodeSet(validators, info.RandomSetSizeWithoutDelta)
	if err != nil {
		return nil, err
	}

	ics.AppendNodeSet(common.NilHash, randomSet)

	return ics, nil
}

// fetchParticipantContextByHash fetches the context info based on the give hash
// and returns a NodeSet which holds the kramaIDs and public keys
func (k *Engine) fetchParticipantContextByHash(ics *types.ICSCommittee, id identifiers.Identifier, hash common.Hash) (
	consensusSet *types.NodeSet,
	consensusNodesHash common.Hash,
	err error,
) {
	consensusNodes, consensusNodesHash, err := k.state.GetConsensusNodes(id, hash)
	if err != nil {
		k.logger.Error("failed to retrieve context nodes", "err", err, "id", id, "hash", hash)

		return nil, common.NilHash, err
	}

	if ics.HasConsensusNodesHash(consensusNodesHash) {
		return nil, consensusNodesHash, nil
	}

	consensusSet = types.NewNodeSet(consensusNodes, uint32(len(consensusNodes)))

	return consensusSet, consensusNodesHash, nil
}
