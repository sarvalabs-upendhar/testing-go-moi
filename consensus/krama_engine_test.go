package consensus

import (
	"testing"

	"github.com/pkg/errors"
	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"

	"github.com/sarvalabs/go-moi/common/tests"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
)

func TestUpdateContextDelta(t *testing.T) {
	addrs := tests.GetAddresses(t, 4)
	kramaIDs := tests.RandomKramaIDs(t, 2)
	operator := kramaIDs[0]
	nodeset := createNodeSet(t, 1, 1, 1, 1, 2, 0)

	createSlot := func(clusterState *ktypes.ClusterState) *ktypes.Slot {
		return ktypes.NewSlot(ktypes.OperatorSlot, clusterState)
	}

	registeredReceiverAddr := addrs[1]
	assetMintIx := createAssetMintIx(t, addrs[0], registeredReceiverAddr)
	clusterState := createTestClusterState(
		t,
		operator,
		kramaIDs[1],
		nodeset,
		common.Interactions{assetMintIx},
		func(clusterState *ktypes.ClusterState) {
			clusterState.AccountInfos[registeredReceiverAddr] = &ktypes.AccountInfo{
				AccType: common.AssetAccount,
			}
		},
	)
	assetMintSlot := createSlot(clusterState)
	assetMintRandomNodes := assetMintSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids

	unregisteredReceiverAddr := addrs[3]
	assetTransferIx := createAssetTransferIx(t, addrs[2], unregisteredReceiverAddr)
	clusterState = createTestClusterState(t, operator, kramaIDs[1], nodeset, assetTransferIx, nil)
	assetTransferSlot := createSlot(clusterState)
	assetTransferRandomNodes := assetTransferSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids

	sm := NewMockStateManager()
	sm.registerAccount(registeredReceiverAddr)

	testcases := []struct {
		name                 string
		sender               identifiers.Address
		receiver             identifiers.Address
		slot                 *ktypes.Slot
		enableDebugMode      bool
		expectedContextDelta common.ContextDelta
		expectedError        error
	}{
		{
			name:          "Slot is nil",
			slot:          nil,
			expectedError: errors.New("nil slot"),
		},
		{
			name:     "Should update context delta if the receiver account is not registered",
			sender:   addrs[2],
			receiver: unregisteredReceiverAddr,
			slot:     assetTransferSlot,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[2]: {
					Role:             common.Sender,
					BehaviouralNodes: []kramaid.KramaID{operator},
					RandomNodes:      []kramaid.KramaID{assetTransferRandomNodes[0]},
				},
				unregisteredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []kramaid.KramaID{assetTransferRandomNodes[0]},
					RandomNodes:      []kramaid.KramaID{assetTransferRandomNodes[1]},
				},
				common.SargaAddress: {
					Role:             common.Genesis,
					BehaviouralNodes: []kramaid.KramaID{operator},
					RandomNodes:      []kramaid.KramaID{assetTransferRandomNodes[0]},
				},
			},
		},
		{
			name:     "Should update context delta if the receiver account is registered",
			sender:   addrs[0],
			receiver: registeredReceiverAddr,
			slot:     assetMintSlot,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: {
					Role:             common.Sender,
					BehaviouralNodes: []kramaid.KramaID{operator},
					RandomNodes:      []kramaid.KramaID{assetMintRandomNodes[0]},
				},
				registeredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []kramaid.KramaID{operator},
					RandomNodes:      []kramaid.KramaID{assetMintRandomNodes[0]},
				},
			},
		},
		{
			name:            "Should update context delta partially if the receiver account is not registered",
			sender:          addrs[2],
			receiver:        unregisteredReceiverAddr,
			slot:            assetTransferSlot,
			enableDebugMode: true,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[2]: {
					Role: common.Sender,
				},
				unregisteredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []kramaid.KramaID{assetTransferRandomNodes[0]},
					RandomNodes:      []kramaid.KramaID{assetTransferRandomNodes[1]},
				},
				common.SargaAddress: {
					Role: common.Genesis,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			engineParams := &createKramaEngineParams{
				sm: sm,
				cfgCallback: func(cfg *config.ConsensusConfig) {
					cfg.EnableDebugMode = test.enableDebugMode
				},
			}

			engine := createTestKramaEngine(t, engineParams)

			err := engine.updateContextDelta(test.slot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			actualContextDelta := test.slot.ClusterState().GetContextDelta()
			checkContextDelta(t, test.sender, test.receiver, test.expectedContextDelta, actualContextDelta)
		})
	}
}

func TestPartiallyUpdateContextDelta(t *testing.T) {
	addrs := tests.GetAddresses(t, 4)
	kramaIDs := tests.RandomKramaIDs(t, 2)
	operator := kramaIDs[0]
	nodeset := createNodeSet(t, 1, 1, 1, 1, 2, 0)

	createSlot := func(clusterState *ktypes.ClusterState) *ktypes.Slot {
		return ktypes.NewSlot(ktypes.OperatorSlot, clusterState)
	}

	registeredReceiverAddr := addrs[1]
	assetMintIx := createAssetMintIx(t, addrs[0], registeredReceiverAddr)
	clusterState := createTestClusterState(
		t,
		operator,
		kramaIDs[1],
		nodeset,
		common.Interactions{assetMintIx},
		func(clusterState *ktypes.ClusterState) {
			clusterState.AccountInfos[registeredReceiverAddr] = &ktypes.AccountInfo{
				AccType: common.AssetAccount,
			}
		},
	)
	assetMintSlot := createSlot(clusterState)

	unregisteredReceiverAddr := addrs[3]
	assetTransferIx := createAssetTransferIx(t, addrs[2], unregisteredReceiverAddr)
	clusterState = createTestClusterState(t, operator, kramaIDs[1], nodeset, assetTransferIx, nil)
	assetTransferSlot := createSlot(clusterState)
	assetTransferRandomNodes := assetTransferSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids

	engineParams := &createKramaEngineParams{
		smCallback: func(sm *MockStateManager) {
			sm.registerAccount(registeredReceiverAddr)
		},
	}

	engine := createTestKramaEngine(t, engineParams)

	testcases := []struct {
		name                 string
		sender               identifiers.Address
		receiver             identifiers.Address
		slot                 *ktypes.Slot
		expectedContextDelta common.ContextDelta
		expectedError        error
	}{
		{
			name:     "Should not update context delta if the receiver account is registered",
			sender:   addrs[0],
			receiver: registeredReceiverAddr,
			slot:     assetMintSlot,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[0]: {
					Role: common.Sender,
				},
				registeredReceiverAddr: {
					Role: common.Receiver,
				},
			},
		},
		{
			name:     "Should update context delta partially if the receiver account is not registered",
			sender:   addrs[2],
			receiver: unregisteredReceiverAddr,
			slot:     assetTransferSlot,
			expectedContextDelta: map[identifiers.Address]*common.DeltaGroup{
				addrs[2]: {
					Role: common.Sender,
				},
				unregisteredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []kramaid.KramaID{assetTransferRandomNodes[0]},
					RandomNodes:      []kramaid.KramaID{assetTransferRandomNodes[1]},
				},
				common.SargaAddress: {
					Role: common.Genesis,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := engine.partiallyUpdateContextDelta(test.slot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			actualContextDelta := test.slot.ClusterState().GetContextDelta()
			checkContextDelta(t, test.sender, test.receiver, test.expectedContextDelta, actualContextDelta)
		})
	}
}
