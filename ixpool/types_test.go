package ixpool

import (
	"testing"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/require"
)

func TestSlotRequired(t *testing.T) {
	testcases := []struct {
		name          string
		ixs           types.Interactions
		expectedSlots uint64
	}{
		{
			name: "slots required for single ixn",
			ixs: types.Interactions{
				newTestInteraction(t, types.IxValueTransfer, 9, tests.RandomAddress(t), nil),
			},
			expectedSlots: 1,
		},
		{
			name: "slots required for multiple ixns",
			ixs: types.Interactions{
				newTestInteraction(t, types.IxValueTransfer, 9, tests.RandomAddress(t), func(ixData *types.IxData) {
					ixData.Input.Payload = make([]byte, ixSlotSize*8)
				}),
				newTestInteraction(t, types.IxValueTransfer, 9, tests.RandomAddress(t), nil),
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
