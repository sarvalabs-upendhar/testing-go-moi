package ixpool

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
)

func TestSlotRequired(t *testing.T) {
	testcases := []struct {
		name          string
		ixs           []*common.Interaction
		expectedSlots uint64
	}{
		{
			name: "slots required for single ixn",
			ixs: []*common.Interaction{
				newTestInteraction(
					t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					9, tests.RandomAddress(t), 0, nil,
				),
			},
			expectedSlots: 1,
		},
		{
			name: "slots required for multiple ixns",
			ixs: []*common.Interaction{
				newTestInteraction(
					t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					9, tests.RandomAddress(t), 0, func(ixData *common.IxData) {
						ixData.IxOps[0].Payload = make([]byte, IxSlotSize*8)
					},
				),
				newTestInteraction(
					t, common.IxAssetCreate, tests.CreateAssetCreatePayload(t),
					9, tests.RandomAddress(t), 0, nil,
				),
			},
			expectedSlots: 10,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			slotCount := slotsRequired(test.ixs...)
			require.Equal(t, test.expectedSlots, slotCount)
		})
	}
}

func TestIncreaseWithinLimit(t *testing.T) {
	testcases := []struct {
		name              string
		slots             uint64
		shouldUpdate      bool
		expectedNewHeight uint64
	}{
		{
			name:              "total slots should be increased",
			slots:             5,
			shouldUpdate:      true,
			expectedNewHeight: 100,
		},
		{
			name:              "total slots shouldn't be increased",
			slots:             6,
			shouldUpdate:      false,
			expectedNewHeight: 95,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			g := slotGauge{
				total: 95,
				max:   100,
			}

			updated := g.increaseWithinLimit(test.slots)
			require.Equal(t, test.shouldUpdate, updated)
			require.Equal(t, test.expectedNewHeight, g.read())
		})
	}
}
