package poorna

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/require"
)

func TestIsInboundConnLimitReached(t *testing.T) {
	tests := []struct {
		name                string
		inboundConnCount    int64
		maxInboundConnCount int64
		expected            bool
	}{
		{
			name:                "Inbound limit not reached",
			inboundConnCount:    5,
			maxInboundConnCount: 10,
			expected:            false,
		},
		{
			name:                "Inbound limit reached",
			inboundConnCount:    10,
			maxInboundConnCount: 10,
			expected:            true,
		},
		{
			name:                "Inbound limit exceeded",
			inboundConnCount:    11,
			maxInboundConnCount: 10,
			expected:            true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connInfo := &ConnectionInfo{
				inboundConnCount:    test.inboundConnCount,
				maxInboundConnCount: test.maxInboundConnCount,
			}

			require.Equal(t, test.expected, connInfo.isInboundConnLimitReached())
		})
	}
}

func TestIsOutboundConnLimitReached(t *testing.T) {
	tests := []struct {
		name                 string
		outboundConnCount    int64
		maxOutboundConnCount int64
		expected             bool
	}{
		{
			name:                 "Outbound limit not reached",
			outboundConnCount:    5,
			maxOutboundConnCount: 10,
			expected:             false,
		},
		{
			name:                 "Outbound limit reached",
			outboundConnCount:    10,
			maxOutboundConnCount: 10,
			expected:             true,
		},
		{
			name:                 "Outbound limit exceeded",
			outboundConnCount:    11,
			maxOutboundConnCount: 10,
			expected:             true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connInfo := &ConnectionInfo{
				outboundConnCount:    test.outboundConnCount,
				maxOutboundConnCount: test.maxOutboundConnCount,
			}

			require.Equal(t, test.expected, connInfo.isOutboundConnLimitReached())
		})
	}
}

func TestUpdateInboundConnCount(t *testing.T) {
	tests := []struct {
		name              string
		delta             int64
		inboundConnCount  int64
		expectedConnCount int64
	}{
		{
			name:              "Positive delta",
			delta:             2,
			inboundConnCount:  3,
			expectedConnCount: 5,
		},
		{
			name:              "Negative delta",
			delta:             -2,
			inboundConnCount:  5,
			expectedConnCount: 3,
		},
		{
			name:              "Zero delta",
			delta:             0,
			inboundConnCount:  7,
			expectedConnCount: 7,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionInfo{inboundConnCount: test.inboundConnCount}
			ci.updateInboundConnCount(test.delta)
			require.Equal(t, test.expectedConnCount, ci.inboundConnCount)
		})
	}
}

func TestUpdateOutboundConnCount(t *testing.T) {
	tests := []struct {
		name              string
		delta             int64
		outboundConnCount int64
		expectedConnCount int64
	}{
		{
			name:              "Positive delta",
			delta:             2,
			outboundConnCount: 10,
			expectedConnCount: 12,
		},
		{
			name:              "Negative delta",
			delta:             -1,
			outboundConnCount: 8,
			expectedConnCount: 7,
		},
		{
			name:              "Zero delta",
			delta:             0,
			outboundConnCount: 4,
			expectedConnCount: 4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionInfo{outboundConnCount: test.outboundConnCount}
			ci.updateOutboundConnCount(test.delta)
			require.Equal(t, test.expectedConnCount, ci.outboundConnCount)
		})
	}
}

func TestUpdateConnCount(t *testing.T) {
	tests := []struct {
		name              string
		direction         network.Direction
		inboundConnCount  int64
		outboundConnCount int64
		delta             int64
		expectedInbound   int64
		expectedOutbound  int64
	}{
		{
			name:              "Increment inbound",
			direction:         network.DirInbound,
			inboundConnCount:  5,
			outboundConnCount: 10,
			delta:             2,
			expectedInbound:   7,
			expectedOutbound:  10,
		},
		{
			name:              "Decrement outbound",
			direction:         network.DirOutbound,
			inboundConnCount:  5,
			outboundConnCount: 10,
			delta:             -2,
			expectedInbound:   5,
			expectedOutbound:  8,
		},
		{
			name:              "Increment outbound",
			direction:         network.DirOutbound,
			inboundConnCount:  0,
			outboundConnCount: 0,
			delta:             2,
			expectedInbound:   0,
			expectedOutbound:  2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ci := &ConnectionInfo{
				inboundConnCount:  test.inboundConnCount,
				outboundConnCount: test.outboundConnCount,
			}

			ci.updateConnCount(test.direction, test.delta)

			require.Equal(t, test.expectedInbound, ci.inboundConnCount)
			require.Equal(t, test.expectedOutbound, ci.outboundConnCount)
		})
	}
}
