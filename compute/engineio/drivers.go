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

// IxnDriver represents an interface that exposes access to interaction
// information such as the fuel price, fuel limit and interaction type
// along with capabilities to make external logic invocations
type IxnDriver interface {
	FuelPrice() *big.Int
	FuelLimit() *big.Int
	IxnType() common.IxType
	Callsite() string
	Calldata() []byte
}

// IxnObject represents a container for Interaction such as its type
// as well as the calldata and callsite information for logic calls
type IxnObject struct {
	data    common.IxData
	payload *common.LogicPayload
}

func (ixn IxnObject) FuelPrice() *big.Int {
	return ixn.data.Input.FuelPrice
}

func (ixn IxnObject) FuelLimit() *big.Int {
	return ixn.data.Input.FuelLimit
}

func (ixn IxnObject) IxnType() common.IxType {
	return ixn.data.Input.Type
}

// NewIxnObject generates a new IxnObject from a common.Interaction value.
func NewIxnObject(ix common.Interaction) *IxnObject {
	logicPayload, err := ix.GetLogicPayload()
	if err != nil {
		return &IxnObject{data: ix.IXData(), payload: &common.LogicPayload{}}
	}

	return &IxnObject{data: ix.IXData(), payload: logicPayload}
}

func (ixn IxnObject) IxType() common.IxType { return ixn.data.Input.Type }
func (ixn IxnObject) Callsite() string      { return ixn.payload.Callsite }
func (ixn IxnObject) Calldata() []byte      { return ixn.payload.Calldata }

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
