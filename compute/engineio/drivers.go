package engineio

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common"
)

// Classdef represents a class definition in a Logic.
// It can be resolved from a string by looking it up on the LogicDriver
type Classdef struct {
	Ptr ElementPtr
}

// IxnObject represents a container for Interaction such as its type
// as well as the calldata and callsite information for logic calls
type IxnObject struct {
	ixtype   common.IxType
	callsite string
	calldata []byte
}

// NewIxnObject generates a new IxnObject from the given types.IxnType, Calldata and Callsite.
func NewIxnObject(kind common.IxType, callsite string, calldata []byte) *IxnObject {
	return &IxnObject{ixtype: kind, callsite: callsite, calldata: calldata}
}

func (ixn IxnObject) IxType() common.IxType { return ixn.ixtype }
func (ixn IxnObject) Callsite() string      { return ixn.callsite }
func (ixn IxnObject) Calldata() []byte      { return ixn.calldata }

// EnvDriver represents an interface that exposes access to environment
// information such as the cluster data, timestamps and fuel prices
// along with capabilities to make external logic invocations
type EnvDriver interface {
	Timestamp() int64
	FuelPrice() *big.Int
	// ClusterInfo() *Cluster
}

// EnvObject represents an environment object which contains values
// obtained from the environment such as timestamp and fuel prices
type EnvObject struct {
	timestamp int64
	fuelPrice *big.Int
}

func (env EnvObject) Timestamp() int64 {
	return env.timestamp
}

func (env EnvObject) FuelPrice() *big.Int {
	return env.fuelPrice
}

// NewEnvObject generates a blank EnvDriver object
func NewEnvObject(timestamp int64, fuelprice *big.Int) *EnvObject {
	return &EnvObject{timestamp: timestamp, fuelPrice: fuelprice}
}
