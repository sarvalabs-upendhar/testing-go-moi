package ixpool

import (
	"context"
	"testing"
	"time"

	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/types"
	"github.com/stretchr/testify/assert"
)

type MockStateManager struct {
	acc map[types.Address]uint64
}

func CreateIxpool(cfgCallback func(cfg *common.IxPoolConfig)) *IxPool {
	ctx := context.Background()
	eventMux := new(utils.TypeMux)
	mockChain := new(MockStateManager)
	cfg := new(common.IxPoolConfig)
	metrics := NilMetrics()

	if cfgCallback != nil {
		cfgCallback(cfg)
	}

	return NewIxPool(ctx, hclog.NewNullLogger(), eventMux, mockChain, cfg, metrics)
}

func (mc *MockStateManager) GetLatestNonce(addr types.Address) (uint64, error) {
	return mc.acc[addr], nil
}

func TestIxPool_IncrementWaitTime_InvalidAccount(t *testing.T) {
	ixPool := CreateIxpool(func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = 100
	})

	err := ixPool.IncrementWaitTime(types.Address{0x0}, 2)
	assert.Error(t, err)
}

func TestIxPool_IncrementWaitTime(t *testing.T) {
	tests := []struct {
		name            string
		addr            types.Address
		delta           int
		shouldReset     bool
		expectedCounter int32
	}{
		{
			name:            "Increment the wait counter by 1",
			addr:            types.Address{0x01},
			delta:           1,
			shouldReset:     false,
			expectedCounter: 1,
		},
		{
			name:            "Increment the wait counter by 2",
			addr:            types.Address{0x02},
			delta:           2,
			shouldReset:     false,
			expectedCounter: 2,
		},

		{
			name:            "wait counter greater than max value ",
			addr:            types.Address{0x03},
			delta:           MaxWaitCounter + 1,
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
				assert.NoError(t, ixPool.IncrementWaitTime(test.addr, 2), 2)
				initTime = time.Now()
			}

			assert.Equal(t, test.expectedCounter, acc.delayCounter)
			if !test.shouldReset {
				assert.InDelta(t, utils.ExponentialTimeout(2, acc.delayCounter), acc.waitTime.Sub(initTime), 400000)
			} else {
				assert.InDelta(t, utils.ExponentialTimeout(2, acc.delayCounter), initTime.Sub(acc.waitTime), 400000)
			}
		})
	}
}

func TestIxPool_ResetWaitTime_WithTesseract(t *testing.T) {
	ts := &types.Tesseract{
		Ixns: types.Interactions{
			&types.Interaction{
				// From: []byte{0x00},
			},
		},
	}

	ixPool := CreateIxpool(func(c *common.IxPoolConfig) {
		c.Mode = 0
		c.PriceLimit = 100
	})
	acc := ixPool.createAccountOnce(types.Address{0x00}, 0)
	acc.incrementCounter(2)
	assert.Equal(t, int32(1), acc.delayCounter)
	ixPool.ResetWithHeaders(ts)
}

// TODO: Add more test cases to check nonce edge conditions
