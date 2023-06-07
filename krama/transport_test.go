package krama

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

// testcase args
type InitClusterCommArgs struct {
	nodeSet  []*types.NodeSet
	slotType ktypes.SlotType
}

func CreateTransport() (*Transport, *MockServer) {
	network := NewMockServer()
	transport := NewKramaTransport(hclog.NewNullLogger(), network)

	return transport, network
}

func CreateInteractions(t *testing.T, sender types.Address) *types.Interactions {
	t.Helper()

	payload, err := polo.Polorize(types.AssetCreatePayload{
		Type:       types.AssetKindValue,
		Symbol:     "GR",
		Supply:     big.NewInt(100),
		Dimension:  1,
		IsLogical:  false,
		IsStateFul: false,
	})
	require.NoError(t, err)

	ixn, err := types.NewInteraction(types.IxData{
		Input: types.IxInput{
			Type:      types.IxAssetCreate,
			Nonce:     0,
			Sender:    sender,
			FuelPrice: big.NewInt(1),
			Payload:   payload,
		},
	}, nil)
	require.NoError(t, err)

	return &types.Interactions{ixn}
}

func CreateSlot(t *testing.T, nodeset []*types.NodeSet, slotType ktypes.SlotType) *ktypes.Slot {
	t.Helper()

	kramaIDs := tests.GetTestKramaIDs(t, 2)
	operator := kramaIDs[0]

	address, err := operator.MoiID()
	require.NoError(t, err)

	ixs := CreateInteractions(t, types.HexToAddress(address))

	clusterID, err := generateClusterID()
	require.NoError(t, err)

	clusterInfo := ktypes.NewICS(6, nil, *ixs, clusterID, operator, time.Now(), kramaIDs[1])
	clusterInfo.NodeSet.Nodes = nodeset

	slot := ktypes.NewSlot(slotType, clusterInfo)

	return slot
}

func Test_InitClusterCommunication_Connect(t *testing.T) {
	testcases := []struct {
		name        string
		args        InitClusterCommArgs
		beforeTest  func(network *MockServer, peers []id.KramaID)
		expected    uint8
		expectedErr error
	}{
		{
			name: fmt.Sprintf(
				"Validator with no connections should establish connection with %v ics nodes",
				MinimumConnectionCount,
			),
			args: InitClusterCommArgs{
				nodeSet: []*types.NodeSet{
					{
						Ids: tests.GetTestKramaIDs(t, 3),
					},
					{
						Ids: tests.GetTestKramaIDs(t, 3),
					},
				},
				slotType: ktypes.ValidatorSlot,
			},
			expected:    3,
			expectedErr: nil,
		},
		{
			name: fmt.Sprintf(
				"Validator with %v existing connections should establish connection with %v ics node",
				2,
				MinimumConnectionCount-2,
			),
			args: InitClusterCommArgs{
				nodeSet: []*types.NodeSet{
					{
						Ids: tests.GetTestKramaIDs(t, 3),
					},
				},
				slotType: ktypes.ValidatorSlot,
			},
			beforeTest: func(network *MockServer, peers []id.KramaID) {
				for _, peer := range peers {
					err := network.ConnectPeer(peer)
					require.NoError(t, err)
				}
			},
			expected:    3,
			expectedErr: nil,
		},
		{
			name: "Operator shouldn't connect with any ics node",
			args: InitClusterCommArgs{
				nodeSet: []*types.NodeSet{
					{
						Ids: tests.GetTestKramaIDs(t, 5),
					},
				},
				slotType: ktypes.OperatorSlot,
			},
			expected:    0,
			expectedErr: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			transport, network := CreateTransport()

			if testcase.beforeTest != nil {
				testcase.beforeTest(network, testcase.args.nodeSet[0].Ids[:2])
			}

			slot := CreateSlot(t, testcase.args.nodeSet, testcase.args.slotType)

			err := transport.InitClusterCommunication(context.Background(), slot)
			if err != nil {
				require.NoError(t, err)
			}

			require.Equal(t, testcase.expectedErr, err)
			// Check whether three random ics nodes are connected
			require.Equal(t, testcase.expected, uint8(len(network.peers)))
		})
	}
}

func Test_InitClusterCommunication_Disconnect(t *testing.T) {
	testcases := []struct {
		name         string
		args         InitClusterCommArgs
		expected     uint8
		expectedConn uint8
		expectedErr  error
	}{
		{
			name: "Validator should disconnect from all the ics nodes",
			args: InitClusterCommArgs{
				nodeSet: []*types.NodeSet{
					{
						Ids: tests.GetTestKramaIDs(t, 3),
					},
					{
						Ids: tests.GetTestKramaIDs(t, 3),
					},
				},
				slotType: ktypes.ValidatorSlot,
			},
			expected:     0,
			expectedConn: 3,
			expectedErr:  nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			transport, network := CreateTransport()
			slot := CreateSlot(t, testcase.args.nodeSet, testcase.args.slotType)

			ctx, cancel := context.WithCancel(context.Background())

			err := transport.InitClusterCommunication(ctx, slot)
			if err != nil {
				require.NoError(t, err)
			}

			require.Equal(t, testcase.expectedErr, err)
			// Check whether three random ics nodes are connected
			require.Equal(t, testcase.expectedConn, uint8(len(network.getPeers())))
			// Disconnect from the connected random ics nodes
			cancel()
			time.Sleep(2 * time.Second)
			// Check whether all the connected random ics nodes are disconnected
			require.Equal(t, testcase.expected, uint8(len(network.getPeers())))
		})
	}
}
