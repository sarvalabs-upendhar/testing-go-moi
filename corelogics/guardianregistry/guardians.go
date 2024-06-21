package guardianregistry

import (
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const (
	SlotGuardians      = 0
	SlotKnownGuardians = 1

	SlotOperators      = 2
	SlotKnownOperators = 3
	SlotMasterOperator = 8

	SlotApproved        = 4
	SlotReferralRewards = 5
	SlotAdministrators  = 9

	SlotNodeLimitKYC = 6
	SlotNodeLimitKYB = 7

	SlotTotalIncentives = 10
)

type Master struct {
	MOIID  string   `polo:"MOIID"`
	Wallet [32]byte `polo:"Wallet"`
	PubKey []byte   `polo:"PubKey"`
}

type Operator struct {
	Identifier   string      `polo:"Identifier"`
	Verification VerifyProof `polo:"Verification"`
	Guardians    []string    `polo:"Guardians"`
}

type VerifyProof struct {
	Kind  string `polo:"Kind"`
	Proof []byte `polo:"Proof"`
}

type Guardian struct {
	KramaID    string    `polo:"KramaID"`
	OperatorID string    `polo:"OperatorID"`
	Incentive  Incentive `polo:"Incentive"`
	PublicKey  []byte    `polo:"PublicKey"`
	ExtraData  []byte    `polo:"ExtraData"`
}

type Incentive struct {
	Amount          uint64   `polo:"Amount"`
	Wallet          [32]byte `polo:"Wallet"`
	ReferralPercent uint64   `polo:"ReferralPercent"`
	ReferralWallet  [32]byte `polo:"ReferralWallet"`
}

type StateObject interface {
	GetStorageEntry(identifiers.LogicID, []byte) ([]byte, error)
}

func GetGuardianPublicKeys(state StateObject, ids ...kramaid.KramaID) ([][]byte, error) {
	pubkeys := make([][]byte, 0, len(ids))

	for _, id := range ids {
		// Encode and hash the krama ID
		encoded, _ := polo.Polorize(id)
		hashed := blake2b.Sum256(encoded)

		// Generate a storage access key for Registry.Guardians[kramaID].PubKey
		key := pisa.GenerateStorageKey(SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(3))

		// Retrieve the value for the storage key
		val, err := state.GetStorageEntry(common.GuardianLogicID, key)
		if err != nil {
			return nil, err
		}

		// Decode the value into some bytes -> public key
		pubkey := make([]byte, 0)
		if err := polo.Depolorize(&pubkey, val); err != nil {
			return nil, err
		}

		pubkeys = append(pubkeys, pubkey)
	}

	return pubkeys, nil
}

func GetGuardianIncentive(state StateObject, id kramaid.KramaID) (uint64, error) {
	encoded, _ := polo.Polorize(id)
	hashed := blake2b.Sum256(encoded)

	// Generate a storage access key for Registry.Guardians[kramaID].PubKey
	key := pisa.GenerateStorageKey(SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(2), pisa.ClsFld(0))

	// Retrieve the value for the storage key
	val, err := state.GetStorageEntry(common.GuardianLogicID, key)
	if err != nil {
		return 0, err
	}

	// Decode the value into uint64 -> amount
	var amount uint64
	if err := polo.Depolorize(&amount, val); err != nil {
		return 0, err
	}

	return amount, nil
}

func GetGuardiansLen(state StateObject) (int, error) {
	// Generate a storage access key for Registry.Guardians
	key := pisa.GenerateStorageKey(SlotGuardians)

	// Retrieve the value for the storage key
	val, err := state.GetStorageEntry(common.GuardianLogicID, key)
	if err != nil {
		return 0, err
	}

	var size int

	if err := polo.Depolorize(&size, val); err != nil {
		return 0, err
	}

	return size, nil
}

func GetTotalIncentives(state StateObject) (uint64, error) {
	// Generate a storage access key for Registry.TotalIncentives
	key := pisa.GenerateStorageKey(SlotTotalIncentives)

	// Retrieve the value for the storage key
	val, err := state.GetStorageEntry(common.GuardianLogicID, key)
	if err != nil {
		return 0, err
	}

	var totalIncentives uint64
	if err := polo.Depolorize(&totalIncentives, val); err != nil {
		return 0, err
	}

	return totalIncentives, nil
}
