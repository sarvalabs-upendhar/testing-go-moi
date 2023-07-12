package ixpool

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
)

func TestSlotRequired(t *testing.T) {
	testcases := []struct {
		name          string
		ixs           common.Interactions
		expectedSlots uint64
	}{
		{
			name: "slots required for single ixn",
			ixs: common.Interactions{
				newTestInteraction(t, common.IxValueTransfer, 9, tests.RandomAddress(t), nil),
			},
			expectedSlots: 1,
		},
		{
			name: "slots required for multiple ixns",
			ixs: common.Interactions{
				newTestInteraction(t, common.IxValueTransfer, 9, tests.RandomAddress(t), func(ixData *common.IxData) {
					ixData.Input.Payload = make([]byte, ixSlotSize*8)
				}),
				newTestInteraction(t, common.IxValueTransfer, 9, tests.RandomAddress(t), nil),
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
