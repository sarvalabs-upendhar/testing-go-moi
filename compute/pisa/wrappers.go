package pisa

import (
	"math/big"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-polo"
)

type Ixn struct {
	driver   engineio.InteractionDriver
	calldata polo.Document
}

func newIxn(driver engineio.InteractionDriver) Ixn {
	calldata := make(polo.Document)
	_ = polo.Depolorize(&calldata, driver.Calldata())

	return Ixn{driver: driver, calldata: calldata}
}

func (ixn Ixn) Kind() string   { return ixn.driver.Type().String() }
func (ixn Ixn) Hash() [32]byte { return ixn.driver.Hash() }

func (ixn Ixn) FuelPrice() *big.Int { return ixn.driver.FuelPrice() }
func (ixn Ixn) FuelLimit() uint64   { return ixn.driver.FuelLimit() }

func (ixn Ixn) Callsite() string        { return ixn.driver.Callsite() }
func (ixn Ixn) Calldata() polo.Document { return ixn.calldata }

type Env struct {
	driver engineio.EnvironmentDriver
}

func (env Env) Timestamp() uint64 { return env.driver.Timestamp() }
func (env Env) ClusterID() string { return env.driver.ClusterID() }

type Crypto int

func (Crypto) ValidateSignature(sig []byte) bool {
	return common.CanUnmarshalSignature(sig)
}

func (Crypto) VerifySignature(data, sig, pub []byte) (bool, error) {
	return crypto.Verify(data, sig, pub)
}
