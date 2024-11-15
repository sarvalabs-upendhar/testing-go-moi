package pisa

import (
	"math/big"

	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-polo"
)

type Txn struct {
	driver   engineio.IxDriver
	calldata polo.Document
}

func newTxn(driver engineio.IxDriver) Txn {
	calldata := make(polo.Document)
	_ = polo.Depolorize(&calldata, driver.Calldata())

	return Txn{driver: driver, calldata: calldata}
}

func (txn Txn) Kind() string   { return txn.driver.Type().String() }
func (txn Txn) Hash() [32]byte { return txn.driver.Hash() }

func (txn Txn) FuelPrice() *big.Int { return txn.driver.FuelPrice() }
func (txn Txn) FuelLimit() uint64   { return txn.driver.FuelLimit() }

func (txn Txn) Callsite() string        { return txn.driver.Callsite() }
func (txn Txn) Calldata() polo.Document { return txn.calldata }

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
