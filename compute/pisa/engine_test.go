package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/drivers"
	"github.com/sarvalabs/go-pisa/logic"
	"github.com/sarvalabs/go-pisa/state"
)

func TestInterfaces(t *testing.T) {
	t.Run("Results", func(t *testing.T) {
		require.Implements(t, (*engineio.ErrorResult)(nil), Error{})
		require.Implements(t, (*engineio.CallResult)(nil), Result{})
	})

	t.Run("Engine", func(t *testing.T) {
		require.Implements(t, (*engineio.Engine)(nil), Engine{})
		require.Implements(t, (*engineio.EngineInstance)(nil), Instance{})
	})

	t.Run("Drivers", func(t *testing.T) {
		require.Implements(t, (*state.Driver)(nil), &State{})
		require.Implements(t, (*logic.Driver)(nil), &Logic{})

		require.Implements(t, (*drivers.Interaction)(nil), Ixn{})
		require.Implements(t, (*drivers.Environment)(nil), Env{})
		require.Implements(t, (*drivers.Cryptography)(nil), Crypto(0))
	})
}

func TestError_Decode(t *testing.T) {
	tests := []*pisa.ErrorResult{
		{Kind: "CallFailure", Error: "failure"},
		{Kind: "RuntimeFailure", Error: "failed", Revert: true, Trace: []string{"foo", "boo"}},
	}

	engine := NewEngine()

	for _, test := range tests {
		errResult, err := engine.DecodeErrorResult(test.Bytes())
		require.NoError(t, err)
		require.Equal(t, test.String(), errResult.String())
	}
}

func TestManifestSerialization(t *testing.T) {
	engineio.RegisterEngine(NewEngine())

	manifest, err := engineio.NewManifestFromFile("./../exlogics/tokenledger/tokenledger.yaml")
	require.NoError(t, err)

	encoded, err := manifest.Encode(common.POLO)
	require.NoError(t, err)

	decoded, err := engineio.NewManifest(encoded, common.POLO)
	require.NoError(t, err)

	require.Equal(t, manifest, decoded)
}
