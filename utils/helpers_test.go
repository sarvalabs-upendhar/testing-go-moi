package utils

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/types"
)

func getLogicID(t *testing.T, logicID string) types.LogicID {
	t.Helper()

	logicID = strings.TrimPrefix(logicID, "0x")

	logicBytes, err := hex.DecodeString(logicID)
	require.NoError(t, err)

	logic := types.LogicID(logicBytes)
	ok := logic.Valid()
	require.True(t, ok)

	return logic
}
