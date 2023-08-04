package engineio

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-moi/common"
)

func TestCallsiteKind_String(t *testing.T) {
	require.True(t, len(callsiteKindFromString) == len(callsiteKindToString))

	require.Equal(t, "invokable", InvokableCallsite.String())
	require.Equal(t, "interactable", InteractableCallsite.String())
	require.Equal(t, "deployer", DeployerCallsite.String())
	require.Equal(t, "enlister", EnlisterCallsite.String())

	require.PanicsWithValue(t, "unknown CallsiteKind variant", func() {
		_ = CallsiteKind(10).String()
	})
}

func TestCallsiteKind_IxnType(t *testing.T) {
	require.Equal(t, common.IxLogicInvoke, InvokableCallsite.IxnType())
	require.Equal(t, common.IxLogicDeploy, DeployerCallsite.IxnType())
	require.Equal(t, common.IxLogicInteract, InteractableCallsite.IxnType())
	require.Equal(t, common.IxLogicEnlist, EnlisterCallsite.IxnType())

	require.PanicsWithValue(t, "unknown CallsiteKind variant", func() {
		CallsiteKind(10).IxnType()
	})
}

//nolint:dupl
func TestCallsite_Serialization(t *testing.T) {
	t.Run("POLO", func(t *testing.T) {
		for kind, str := range callsiteKindToString {
			encoded, err := polo.Polorize(kind)

			require.Nil(t, err)
			require.Equal(t, bytes.Join([][]byte{{6}, []byte(str)}, []byte{}), encoded)

			decoded := new(CallsiteKind)
			err = polo.Depolorize(decoded, encoded)

			require.Nil(t, err)
			require.Equal(t, kind, *decoded)
		}

		require.PanicsWithValue(t, "unknown CallsiteKind variant", func() {
			_, _ = polo.Polorize(CallsiteKind(10))
		})

		malformed := bytes.Join([][]byte{{6}, []byte("malformed")}, []byte{})
		err := polo.Depolorize(new(CallsiteKind), malformed)
		require.EqualError(t, err, "invalid CallsiteKind value")
	})

	t.Run("JSON", func(t *testing.T) {
		for kind, str := range callsiteKindToString {
			encoded, err := json.Marshal(kind)

			require.Nil(t, err)
			require.Equal(t, bytes.Join([][]byte{{34}, []byte(str), {34}}, []byte{}), encoded)

			decoded := new(CallsiteKind)
			err = json.Unmarshal(encoded, decoded)

			require.Nil(t, err)
			require.Equal(t, kind, *decoded)
		}

		require.PanicsWithValue(t, "unknown CallsiteKind variant", func() {
			_, _ = json.Marshal(CallsiteKind(10))
		})

		malformed := bytes.Join([][]byte{{34}, []byte("malformed"), {34}}, []byte{})
		err := json.Unmarshal(malformed, new(CallsiteKind))
		require.EqualError(t, err, "invalid CallsiteKind value")
	})

	t.Run("YAML", func(t *testing.T) {
		for kind, str := range callsiteKindToString {
			encoded, err := yaml.Marshal(kind)

			require.Nil(t, err)
			require.Equal(t, bytes.Join([][]byte{[]byte(str), {10}}, []byte{}), encoded)

			decoded := new(CallsiteKind)
			err = yaml.Unmarshal(encoded, decoded)

			require.Nil(t, err)
			require.Equal(t, kind, *decoded)
		}

		require.PanicsWithValue(t, "unknown CallsiteKind variant", func() {
			_, _ = yaml.Marshal(CallsiteKind(10))
		})

		malformed := bytes.Join([][]byte{{34}, []byte("malformed"), {34}}, []byte{})
		err := yaml.Unmarshal(malformed, new(CallsiteKind))
		require.EqualError(t, err, "invalid CallsiteKind value")
	})
}

func TestCallResult_Ok(t *testing.T) {
	result1 := &CallResult{}
	require.True(t, result1.Ok())

	result2 := &CallResult{Error: []byte{6, 65}}
	require.False(t, result2.Ok())
}
