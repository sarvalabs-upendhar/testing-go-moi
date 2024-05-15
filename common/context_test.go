package common_test

import (
	"reflect"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestCopyContextDelta(t *testing.T) {
	contextDelta := make(common.ContextDelta)
	address := tests.RandomAddress(t)

	contextDelta[address] = &common.DeltaGroup{
		BehaviouralNodes: tests.RandomKramaIDs(t, 2),
		RandomNodes:      tests.RandomKramaIDs(t, 2),
		ReplacedNodes:    tests.RandomKramaIDs(t, 2),
	}

	testcases := []struct {
		name         string
		contextDelta common.ContextDelta
	}{
		{
			name:         "copy context delta",
			contextDelta: contextDelta,
		},
		{
			name: "empty nodes",
			contextDelta: map[identifiers.Address]*common.DeltaGroup{
				address: {},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCtxDelta := test.contextDelta

			ctxDelta := test.contextDelta.Copy()

			require.Equal(t, expectedCtxDelta, ctxDelta)

			if len(expectedCtxDelta[address].BehaviouralNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(test.contextDelta[address].BehaviouralNodes).Pointer(),
					reflect.ValueOf(ctxDelta[address].BehaviouralNodes).Pointer(),
				)
			}

			if len(expectedCtxDelta[address].RandomNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(test.contextDelta[address].RandomNodes).Pointer(),
					reflect.ValueOf(ctxDelta[address].RandomNodes).Pointer(),
				)
			}

			if len(expectedCtxDelta[address].ReplacedNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(test.contextDelta[address].ReplacedNodes).Pointer(),
					reflect.ValueOf(ctxDelta[address].ReplacedNodes).Pointer(),
				)
			}
		})
	}
}
