package ixpool

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
)

type MockStateManager struct {
	acc map[ktypes.Address]uint64
}

func CreateIxpool(cfgCallback func(cfg *common.IxPoolConfig)) *IxPool {
	ctx := context.Background()
	eventMux := new(kutils.TypeMux)
	mockChain := new(MockStateManager)
	cfg := new(common.IxPoolConfig)

	if cfgCallback != nil {
		cfgCallback(cfg)
	}

	return NewIxPool(ctx, hclog.NewNullLogger(), eventMux, mockChain, cfg)
}

func (mc *MockStateManager) GetLatestNonce(addr ktypes.Address) (uint64, error) {
	return mc.acc[addr], nil
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	ixPool := CreateIxpool(func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = 100
	})

	err := ixPool.IncrementWaitTime(ktypes.Address{0x0})
	assert.Error(t, err)
}

func TestIxPool_IncrementWaitTime(t *testing.T) {
	tests := []struct {
		name            string
		addr            ktypes.Address
		delta           int
		shouldReset     bool
		expectedCounter int32
	}{{
		name:            "Increment the wait counter by 1",
		addr:            ktypes.Address{0x01},
		delta:           1,
		shouldReset:     false,
		expectedCounter: 1,
	}, {
		name:            "Increment the wait counter by 2",
		addr:            ktypes.Address{0x02},
		delta:           2,
		shouldReset:     false,
		expectedCounter: 2,
	},

		{
			name:            "wait counter greater than max value ",
			addr:            ktypes.Address{0x03},
			delta:           11,
			shouldReset:     true,
			expectedCounter: 0,
		},
	}

	ixPool := CreateIxpool(func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = 100
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			acc := ixPool.createAccountOnce(test.addr, 0)
			var initTime time.Time
			for i := 0; i < test.delta; i++ {
				assert.NoError(t, ixPool.IncrementWaitTime(test.addr))
				initTime = time.Now()
			}
			assert.Equal(t, acc.delayCounter, test.expectedCounter)
			if !test.shouldReset {
				assert.InDelta(t, kutils.ExponentialTimeout(acc.delayCounter), acc.waitTime.Sub(initTime), 400000)
			} else {
				assert.InDelta(t, kutils.ExponentialTimeout(acc.delayCounter), initTime.Sub(acc.waitTime), 400000)
			}
		})
	}
}

func TestIxPool_ResetWaitTime_WithTesseract(t *testing.T) {
	ts := &ktypes.Tesseract{
		Body: ktypes.TesseractBody{
			Interactions: ktypes.Interactions{
				&ktypes.Interaction{
					//From: []byte{0x00},
				},
			},
		},
	}

	ixPool := CreateIxpool(func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = 100
	})
	acc := ixPool.createAccountOnce(ktypes.Address{0x00}, 0)
	acc.incrementCounter()
	assert.Equal(t, int32(1), acc.delayCounter)
	ixPool.ResetWithHeaders(ts)
	assert.Equal(t, int32(0), acc.delayCounter)
}

//TODO: Add more test cases to check nonce edge conditions
