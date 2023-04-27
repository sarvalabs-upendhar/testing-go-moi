package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestContinuity(t *testing.T) {
	t.Run("continueOk", func(t *testing.T) {
		fuel := engineio.Fuel(10)
		cont := continueOk{consumed: fuel}

		assert.Equal(t, continueModeOk, cont.mode())
		assert.Equal(t, fuel, cont.fuel())
	})

	t.Run("continueTerm", func(t *testing.T) {
		cont := continueTerm{}

		assert.Equal(t, continueModeTerm, cont.mode())
		assert.Equal(t, engineio.Fuel(0), cont.fuel())
	})

	t.Run("continueJump", func(t *testing.T) {
		fuel := engineio.Fuel(10)
		dest := uint64(12345)
		cont := continueJump{consumed: fuel, jumpdest: dest}

		assert.Equal(t, continueModeJump, cont.mode())
		assert.Equal(t, fuel, cont.fuel())
		assert.Equal(t, dest, cont.jumpdest)
	})

	t.Run("raiseException", func(t *testing.T) {
		except := &Exception{Class: RuntimeError.Name(), Data: "test exception"}
		cont := raise(except)

		require.NotNil(t, cont)
		assert.Equal(t, continueModeExcept, cont.mode())
		assert.Equal(t, engineio.Fuel(0), cont.fuel())
		assert.Equal(t, except, cont.exception)
	})

	t.Run("raiseExceptionWithConsumption", func(t *testing.T) {
		except := &Exception{Class: RuntimeError.Name(), Data: "test exception"}
		cont := raise(except).withConsumption(42)

		require.NotNil(t, cont)
		assert.Equal(t, continueModeExcept, cont.mode())
		assert.Equal(t, engineio.Fuel(42), cont.fuel())
		assert.Equal(t, except, cont.exception)
	})
}
