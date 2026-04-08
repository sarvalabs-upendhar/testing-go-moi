package pisa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

func TestInterfaces(t *testing.T) {
	t.Run("Runtime", func(t *testing.T) {
		require.Implements(t, (*engineio.Runtime)(nil), new(RuntimeWrapper))
	})

	t.Run("Engine", func(t *testing.T) {
		require.Implements(t, (*engineio.Engine)(nil), new(Pisa))
	})

	t.Run("Action", func(t *testing.T) {
		require.Implements(t, (*engineio.Action)(nil), new(common.IxOp))
	})
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
