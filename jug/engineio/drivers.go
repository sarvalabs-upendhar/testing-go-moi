package engineio

import (
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/types"
)

// EnvDriver represents an interface that exposes access to environment
// information such as the cluster data, timestamps and fuel prices
// along with capabilities to make external logic invocations
type EnvDriver interface{}

// NewEnvDriver generates a blank EnvDriver object
func NewEnvDriver() EnvDriver {
	return nil
}

// IxnObject represents a container for Interaction such as its type
// as well as the calldata and callsite information for logic calls
type IxnObject struct {
	ixtype   types.IxType
	callsite string
	calldata polo.Document
}

// NewIxnObject generates a new IxnObject from the given types.IxnType, Calldata and Callsite.
func NewIxnObject(kind types.IxType, callsite string, calldata polo.Document) *IxnObject {
	return &IxnObject{ixtype: kind, callsite: callsite, calldata: calldata}
}

func (ixn IxnObject) IxType() types.IxType    { return ixn.ixtype }
func (ixn IxnObject) Callsite() string        { return ixn.callsite }
func (ixn IxnObject) Calldata() polo.Document { return ixn.calldata }
