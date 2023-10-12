package consensus

import (
	"testing"

	"github.com/sarvalabs/go-moi/common/config"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/tests"

	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/stretchr/testify/require"
)

func TestUpdateContextDelta(t *testing.T) {
	addrs := tests.GetAddresses(t, 3)
	kramaIDs := tests.GetTestKramaIDs(t, 2)

	registeredReceiverSlot := createSlot(
		t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[0],
		common.NilAddress,
		ktypes.OperatorSlot,
		false,
	)

	unregisteredReceiverSlot := createSlot(
		t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[1],
		addrs[2],
		ktypes.OperatorSlot,
		true,
	)

	registeredReceiverRandomNodes := registeredReceiverSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids
	registeredReceiverAddr := registeredReceiverSlot.ClusterState().Ixs[0].Receiver()

	unregisteredReceiverRandomNodes := unregisteredReceiverSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids
	unregisteredReceiverAddr := unregisteredReceiverSlot.ClusterState().Ixs[0].Receiver()

	sm := NewMockStateManager()
	sm.registerAccount(registeredReceiverAddr)

	testcases := []struct {
		name                 string
		sender               common.Address
		receiver             common.Address
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
			sender:   addrs[1],
			receiver: addrs[2],
			slot:     unregisteredReceiverSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[1]: {
					Role:             common.Sender,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{unregisteredReceiverRandomNodes[0]},
				},
				addrs[2]: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{unregisteredReceiverRandomNodes[0]},
					RandomNodes:      []id.KramaID{unregisteredReceiverRandomNodes[1]},
				},
				common.SargaAddress: {
					Role:             common.Genesis,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{unregisteredReceiverRandomNodes[0]},
				},
			},
		},
		{
			name:     "Should update context delta if the receiver account is registered",
			sender:   addrs[0],
			receiver: registeredReceiverAddr,
			slot:     registeredReceiverSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[0]: {
					Role:             common.Sender,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{registeredReceiverRandomNodes[0]},
				},
				registeredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{registeredReceiverRandomNodes[0]},
				},
			},
		},
		{
			name:            "Should update context delta partially if the receiver account is not registered",
			sender:          addrs[1],
			receiver:        unregisteredReceiverAddr,
			slot:            unregisteredReceiverSlot,
			enableDebugMode: true,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[1]: {
					Role: common.Sender,
				},
				unregisteredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{unregisteredReceiverRandomNodes[0]},
					RandomNodes:      []id.KramaID{unregisteredReceiverRandomNodes[1]},
				},
				common.SargaAddress: {
					Role: common.Genesis,
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			engine := createTestKramaEngine(t, sm, func(cfg *config.ConsensusConfig) {
				cfg.EnableDebugMode = test.enableDebugMode
			})

			err := engine.updateContextDelta(test.slot)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)

			actualContextDelta := test.slot.ClusterState().GetContextDelta()

			require.Equal(t, test.expectedContextDelta[test.sender], actualContextDelta[test.sender])
			require.Equal(t, test.expectedContextDelta[test.receiver], actualContextDelta[test.receiver])
			require.Equal(t, test.expectedContextDelta[common.SargaAddress], actualContextDelta[common.SargaAddress])
		})
	}
}

func TestPartiallyUpdateContextDelta(t *testing.T) {
	addrs := tests.GetAddresses(t, 3)
	kramaIDs := tests.GetTestKramaIDs(t, 2)

	registeredReceiverSlot := createSlot(t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[0],
		common.NilAddress,
		ktypes.OperatorSlot,
		false,
	)

	unregisteredReceiverSlot := createSlot(
		t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[1],
		addrs[2],
		ktypes.OperatorSlot,
		true,
	)

	registeredReceiverAddr := registeredReceiverSlot.ClusterState().Ixs[0].Receiver()
	unregisteredReceiverAddr := unregisteredReceiverSlot.ClusterState().Ixs[0].Receiver()
	unregisteredReceiverRandomNodes := unregisteredReceiverSlot.ClusterState().NodeSet.Nodes[common.RandomSet].Ids

	sm := NewMockStateManager()
	sm.registerAccount(registeredReceiverAddr)

	engine := createTestKramaEngine(t, sm, nil)

	testcases := []struct {
		name                 string
		sender               common.Address
		receiver             common.Address
		slot                 *ktypes.Slot
		expectedContextDelta common.ContextDelta
		expectedError        error
	}{
		{
			name:     "Should not update context delta if the receiver account is registered",
			receiver: registeredReceiverAddr,
			slot:     registeredReceiverSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
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
			receiver: unregisteredReceiverAddr,
			slot:     unregisteredReceiverSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[1]: {
					Role: common.Sender,
				},
				unregisteredReceiverAddr: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{unregisteredReceiverRandomNodes[0]},
					RandomNodes:      []id.KramaID{unregisteredReceiverRandomNodes[1]},
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

			require.Equal(t, test.expectedContextDelta[test.sender], actualContextDelta[test.sender])
			require.Equal(t, test.expectedContextDelta[test.receiver], actualContextDelta[test.receiver])
			require.Equal(t, test.expectedContextDelta[common.SargaAddress], actualContextDelta[common.SargaAddress])
		})
	}
}
