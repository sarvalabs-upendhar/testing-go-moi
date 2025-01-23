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
	id := tests.RandomIdentifier(t)

	contextDelta[id] = &common.DeltaGroup{
		ConsensusNodes: tests.RandomKramaIDs(t, 2),
		ReplacedNodes:  tests.RandomKramaIDs(t, 2),
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
			contextDelta: map[identifiers.Identifier]*common.DeltaGroup{
				id: {},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCtxDelta := test.contextDelta

			ctxDelta := test.contextDelta.Copy()

			require.Equal(t, expectedCtxDelta, ctxDelta)

			if len(expectedCtxDelta[id].ConsensusNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(test.contextDelta[id].ConsensusNodes).Pointer(),
					reflect.ValueOf(ctxDelta[id].ConsensusNodes).Pointer(),
				)
			}

			if len(expectedCtxDelta[id].ReplacedNodes) > 0 {
				require.NotEqual(t,
					reflect.ValueOf(test.contextDelta[id].ReplacedNodes).Pointer(),
					reflect.ValueOf(ctxDelta[id].ReplacedNodes).Pointer(),
				)
			}
		})
	}
}
