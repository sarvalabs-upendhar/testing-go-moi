package consensus

import (
	"testing"

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

	receiverRegisteredSlot := createSlot(
		t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[0],
		common.NilAddress,
		ktypes.OperatorSlot,
		false,
	)
	csReceiverRegistered := receiverRegisteredSlot.ClusterState()
	randomNodesReceiverRegistered := csReceiverRegistered.NodeSet.Nodes[common.RandomSet].Ids
	receiverAddress := csReceiverRegistered.Ixs[0].Receiver()

	receiverNotRegisteredSlot := createSlot(
		t,
		kramaIDs[0],
		kramaIDs[1],
		addrs[1],
		addrs[2],
		ktypes.OperatorSlot,
		true,
	)
	csReceiverNotRegistered := receiverNotRegisteredSlot.ClusterState()
	randomNodesReceiverNotRegistered := csReceiverNotRegistered.NodeSet.Nodes[common.RandomSet].Ids

	sm := NewMockStateManager()
	sm.registerAccount(receiverAddress)
	engine := createTestKramaEngine(t, sm)

	testcases := []struct {
		name                 string
		sender               common.Address
		receiver             common.Address
		slot                 *ktypes.Slot
		expectedContextDelta common.ContextDelta
		expectedError        error
	}{
		{
			name:          "slot is nil",
			slot:          nil,
			expectedError: errors.New("nil slot"),
		},
		{
			name:     "context delta updated successfully when receiver account not registered",
			sender:   addrs[1],
			receiver: addrs[2],
			slot:     receiverNotRegisteredSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[1]: {
					Role:             common.Sender,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{randomNodesReceiverNotRegistered[0]},
				},
				addrs[2]: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{randomNodesReceiverNotRegistered[0]},
					RandomNodes:      []id.KramaID{randomNodesReceiverNotRegistered[1]},
				},
				common.SargaAddress: {
					Role:             common.Genesis,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{randomNodesReceiverNotRegistered[0]},
				},
			},
		},
		{
			name:     "context delta updated successfully when receiver account is registered",
			sender:   addrs[0],
			receiver: receiverAddress,
			slot:     receiverRegisteredSlot,
			expectedContextDelta: map[common.Address]*common.DeltaGroup{
				addrs[0]: {
					Role:             common.Sender,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{randomNodesReceiverRegistered[0]},
				},
				receiverAddress: {
					Role:             common.Receiver,
					BehaviouralNodes: []id.KramaID{kramaIDs[0]},
					RandomNodes:      []id.KramaID{randomNodesReceiverRegistered[0]},
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
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
