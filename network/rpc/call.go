package rpc

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Call represents an active RPC. Calls are used to indicate completion
// of RPC requests and are returned within the provided channel in
// the Go() functions.
type Call struct {
	ctx    context.Context
	cancel func()

	finishedMu sync.RWMutex
	finished   bool

	Dest          peer.ID
	SvcID         ServiceID     // The name of the service and method to call.
	Args          interface{}   // The argument to the function.
	Reply         interface{}   // The reply from the function.
	StreamArgs    reflect.Value // streaming objects (channel).
	StreamReplies reflect.Value // streaming replies (channel).
	Done          chan *Call    // Strobes when call is complete.

	errorMu sync.Mutex
	Error   error // After completion, the error status.
	logger  hclog.Logger
}

// newCall panics if arguments are not as expected.
func newCall(
	logger hclog.Logger,
	ctx context.Context,
	dest peer.ID, svcName,
	svcMethod string,
	args,
	reply interface{},
	done chan *Call,
) *Call {
	sID := ServiceID{svcName, svcMethod}

	if !isExportedOrBuiltinType(reflect.TypeOf(args)) {
		panic(fmt.Sprintf("%s: method argument is not exported or builtin", sID))
	}

	if !isExportedOrBuiltinType(reflect.TypeOf(args)) {
		panic(fmt.Sprintf("%s: method reply argument is not exported or builtin", sID))
	}

	if reply == nil || reflect.TypeOf(reply).Kind() != reflect.Ptr {
		panic(fmt.Sprintf("%s: reply type must be a pointer to a type", sID))
	}

	ctx2, cancel := context.WithCancel(ctx)

	return &Call{
		ctx:    ctx2,
		cancel: cancel,
		Dest:   dest,
		SvcID:  sID,
		Args:   args,
		Reply:  reply,
		Error:  nil,
		Done:   done,
		logger: logger.Named("MOIRPC-NewCall"),
	}
}

// newStreamingCall panics if arguments are not as expected.
func newStreamingCall(
	logger hclog.Logger,
	ctx context.Context,
	dest peer.ID,
	svcName,
	svcMethod string,
	streamArgs,
	streamReplies reflect.Value,
	done chan *Call,
) *Call {
	sID := ServiceID{svcName, svcMethod}

	checkChanTypesValid(sID, streamArgs, reflect.RecvDir)
	checkChanTypesValid(sID, streamReplies, reflect.SendDir)

	ctx2, cancel := context.WithCancel(ctx)

	return &Call{
		ctx:           ctx2,
		cancel:        cancel,
		Dest:          dest,
		SvcID:         sID,
		StreamArgs:    streamArgs,
		StreamReplies: streamReplies,
		Error:         nil,
		Done:          done,
		logger:        logger.Named("MOIRPC-NewStreamingCall"),
	}
}

// done places the completed call in the done channel.
func (call *Call) done() {
	call.finishedMu.Lock()
	call.finished = true
	call.finishedMu.Unlock()

	select {
	case call.Done <- call:
		// ok
	default:
		call.logger.Debug("Discarding call reply", "service-ID", call.SvcID)
	}
	call.cancel()
}

func (call *Call) doneWithError(err error) {
	if err != nil {
		call.logger.Error("Setting call error", "err", err)
		call.setError(err)
	}

	call.done()
}

func (call *Call) isFinished() bool {
	call.finishedMu.RLock()
	defer call.finishedMu.RUnlock()

	return call.finished
}

// watch context will wait for a context cancellation
// and close the stream.
func (call *Call) watchContextWithStream(s network.Stream) {
	<-call.ctx.Done()

	if !call.isFinished() { // context was cancelled not by us
		call.logger.Debug("Call context is done before finishing")
		call.doneWithError(call.ctx.Err())
		// This used to be s.Close() But for streaming we definitely
		// need to signal an abnormal finalization of the call when a
		// context is cancelled.
		err := s.Reset()
		if err != nil {
			call.logger.Error("Failed to close stream", "err", err)
		}
	}
}

func (call *Call) setError(err error) {
	call.errorMu.Lock()
	defer call.errorMu.Unlock()

	if call.Error == nil {
		call.Error = err
	}
}

func (call *Call) getError() error {
	call.errorMu.Lock()
	defer call.errorMu.Unlock()

	return call.Error
}

// panics otherwise
func checkChanTypesValid(sID ServiceID, vChan reflect.Value, dir reflect.ChanDir) {
	desc := "argument"
	if dir == reflect.SendDir {
		desc = "reply"
	}

	if vChan.Kind() != reflect.Chan {
		panic(fmt.Sprintf("%s: %s type must be a channel", desc, sID))
	}

	if vChan.Type().ChanDir()&dir == 0 {
		panic(fmt.Sprintf("%s: %s channel has wrong channel direction", sID, desc))
	}

	if !isExportedOrBuiltinType(vChan.Type().Elem()) {
		panic(fmt.Sprintf("%s: %s channel type is not exported or builtin", sID, desc))
	}
}
