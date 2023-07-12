package p2p

import (
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/network"
)

// ConnectionInfo maintains information about current and maximum inbound and outbound connections.
type ConnectionInfo struct {
	inboundConnCount     int64
	outboundConnCount    int64
	maxInboundConnCount  int64
	maxOutboundConnCount int64
}

// NewConnectionInfo returns a new instance of ConnectionInfo with the given max inbound and max outbound connections.
func NewConnectionInfo(maxInboundConnCount int64, maxOutboundConnCount int64) *ConnectionInfo {
	return &ConnectionInfo{
		inboundConnCount:     0,
		outboundConnCount:    0,
		maxInboundConnCount:  maxInboundConnCount,
		maxOutboundConnCount: maxOutboundConnCount,
	}
}

// getInboundConnCount returns the number of active inbound connections.
func (ci *ConnectionInfo) getInboundConnCount() int64 {
	return atomic.LoadInt64(&ci.inboundConnCount)
}

// getOutboundConnCount returns the number of active outbound connections.
func (ci *ConnectionInfo) getOutboundConnCount() int64 {
	return atomic.LoadInt64(&ci.outboundConnCount)
}

// getMaxInboundConnCount returns the maximum number of inbound connections.
func (ci *ConnectionInfo) getMaxInboundConnCount() int64 {
	return ci.maxInboundConnCount
}

// getMaxOutboundConnCount returns the maximum number of outbound connections.
func (ci *ConnectionInfo) getMaxOutboundConnCount() int64 {
	return ci.maxOutboundConnCount
}

// isInboundConnLimitReached returns true if the inbound connection count exceeds the limit.
func (ci *ConnectionInfo) isInboundConnLimitReached() bool {
	return ci.getInboundConnCount() >= ci.getMaxInboundConnCount()
}

// isOutboundConnLimitReached returns true if the outbound connection count exceeds the limit.
func (ci *ConnectionInfo) isOutboundConnLimitReached() bool {
	return ci.getOutboundConnCount() >= ci.getMaxOutboundConnCount()
}

// updateInboundConnCount increments the inbound connection count by the specified delta value.
func (ci *ConnectionInfo) updateInboundConnCount(delta int64) {
	atomic.AddInt64(&ci.inboundConnCount, delta)
}

// updateOutboundConnCount increments the outbound connection count by the specified delta value.
func (ci *ConnectionInfo) updateOutboundConnCount(delta int64) {
	atomic.AddInt64(&ci.outboundConnCount, delta)
}

// updateConnCount updates the inbound and outbound connection counts by the specified delta,
// based on the given direction.
func (ci *ConnectionInfo) updateConnCount(direction network.Direction, delta int64) {
	switch direction {
	case network.DirInbound:
		ci.updateInboundConnCount(delta)
	case network.DirOutbound:
		ci.updateOutboundConnCount(delta)
	}
}
