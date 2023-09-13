package compute

import (
	"testing"

	"github.com/sarvalabs/go-moi-engineio"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/state"
)

func TestEngineIOInterfaces(t *testing.T) {
	require.Implements(t, (*engineio.IxnType)(nil), common.IxType(0))
	require.Implements(t, (*engineio.LogicID)(nil), common.LogicID(""))
	require.Implements(t, (*engineio.Logic)(nil), &state.LogicObject{})
	require.Implements(t, (*engineio.CryptoDriver)(nil), crypto.Cryptographer(0))
	require.Implements(t, (*engineio.IxnDriver)(nil), &common.Interaction{})
	require.Implements(t, (*engineio.CtxDriver)(nil), &state.LogicContextObject{})
	require.Implements(t, (*engineio.EnvDriver)(nil), &common.ExecutionContext{})
}
