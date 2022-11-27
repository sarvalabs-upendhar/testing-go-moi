package krama

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/moichain/common/tests"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

// testcase args
type InitClusterCommArgs struct {
	nodeSet  []*ktypes.NodeSet
	slotType ktypes.SlotType
}

func CreateTransport() (*Transport, *MockServer) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "MOI",
		Level: hclog.LevelFromString("TRACE"),
	})
	network := NewMockServer()
	transport := NewKramaTransport(logger, network)

	return transport, network
}

func CreateInteractions(t *testing.T, sender types.Address) *types.Interactions {
	t.Helper()

	// Construct the interactions data
	ixns := make(types.Interactions, 1)

	ixns[0] = &types.Interaction{
		Data: types.IxData{
			Input: types.InteractionInput{
				Type:     1,
				Nonce:    0,
				From:     sender,
				AnuPrice: 1000,
				Payload: types.InteractionInputPayload{
					AssetData: types.AssetDataInput{
						Symbol:      "GR",
						TotalSupply: 100,
						IsFungible:  true,
						IsMintable:  false,
						Dimension:   1,
					},
				},
			},
		},
	}

	return &ixns
}

func CreateSlot(t *testing.T, nodeset []*ktypes.NodeSet, slotType ktypes.SlotType) *ktypes.Slot {
	t.Helper()

	kramaIDs := tests.GetTestKramaIDs(t, 1)
	operator := kramaIDs[0]

	address, err := operator.MoiID()
	require.NoError(t, err)

	ixs := CreateInteractions(t, types.HexToAddress(address))

	clusterID, err := generateClusterID(operator, *ixs)
	require.NoError(t, err)

	clusterInfo := ktypes.NewICS(6, *ixs, clusterID, operator, time.Now())

	clusterInfo.ICS.Nodes = nodeset

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
				nodeSet: []*ktypes.NodeSet{
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
				nodeSet: []*ktypes.NodeSet{
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
				nodeSet: []*ktypes.NodeSet{
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
				nodeSet: []*ktypes.NodeSet{
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
			require.Equal(t, testcase.expectedConn, uint8(len(network.peers)))
			// Disconnect from the connected random ics nodes
			cancel()
			time.Sleep(2 * time.Second)
			// Check whether all the connected random ics nodes are disconnected
			require.Equal(t, testcase.expected, uint8(len(network.peers)))
		})
	}
}
