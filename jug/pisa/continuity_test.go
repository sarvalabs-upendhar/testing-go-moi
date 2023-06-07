package pisa

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sarvalabs/moichain/jug/engineio"
)

func TestContinuity(t *testing.T) {
	t.Run("continueOk", func(t *testing.T) {
		cont := continueOk{consumed: 10}
		assert.Equal(t, continueModeOk, cont.mode())
		assert.Equal(t, engineio.NewFuel(10), cont.fuel())
	})

	t.Run("continueTerm", func(t *testing.T) {
		cont := continueTerm{}
		assert.Equal(t, continueModeTerm, cont.mode())
		assert.Equal(t, engineio.NewFuel(0), cont.fuel())
	})

	t.Run("continueJump", func(t *testing.T) {
		dest := uint64(12345)
		cont := continueJump{consumed: 10, jumpdest: dest}

		assert.Equal(t, continueModeJump, cont.mode())
		assert.Equal(t, engineio.NewFuel(10), cont.fuel())
		assert.Equal(t, dest, cont.jumpdest)
	})
}
