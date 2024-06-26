package guardianregistry

import (
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

const (
	SlotGuardians      = 0
	SlotKnownGuardians = 1

	SlotOperators      = 2
	SlotKnownOperators = 3
	SlotMasterOperator = 8

	SlotApproved       = 4
	SlotAdministrators = 9

	SlotReferralRewards = 5
	SlotTotalIncentives = 10

	SlotNodeLimitKYC = 6
	SlotNodeLimitKYB = 7
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

func GetGuardianPublicKeys(storage engineio.StorageReader, ids ...kramaid.KramaID) ([][]byte, error) {
	pubkeys := make([][]byte, 0, len(ids))

	for _, id := range ids {
		// Encode and hash the krama ID
		encoded, _ := polo.Polorize(id)
		hashed := blake2b.Sum256(encoded)

		// Generate a storage access key for Registry.Guardians[kramaID].PubKey
		key := pisa.GenerateStorageKey(SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(3))

		// Retrieve the value for the storage key
		val, err := storage.GetStorageEntry(key)
		if err != nil {
			return nil, err
		}

		// Decode the value into some bytes -> public key
		pubkey := make([]byte, 0)
		if err = polo.Depolorize(&pubkey, val); err != nil {
			return nil, err
		}

		pubkeys = append(pubkeys, pubkey)
	}

	return pubkeys, nil
}

func GetGuardianIncentive(storage engineio.StorageReader, id kramaid.KramaID) (uint64, error) {
	encoded, _ := polo.Polorize(id)
	hashed := blake2b.Sum256(encoded)

	// Generate a storage access key for Registry.Guardians[kramaID].PubKey
	key := pisa.GenerateStorageKey(SlotGuardians, pisa.MapKey(hashed), pisa.ClsFld(2), pisa.ClsFld(0))

	// Retrieve the value for the storage key
	val, err := storage.GetStorageEntry(key)
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

func GetGuardiansCount(storage engineio.StorageReader) (int, error) {
	// Generate a storage access key for Registry.Guardians
	key := pisa.GenerateStorageKey(SlotGuardians)

	// Retrieve the value for the storage key
	val, err := storage.GetStorageEntry(key)
	if err != nil {
		return 0, err
	}

	var size int
	if err = polo.Depolorize(&size, val); err != nil {
		return 0, err
	}

	return size, nil
}

func GetTotalIncentives(storage engineio.StorageReader) (uint64, error) {
	// Generate a storage access key for Registry.TotalIncentives
	key := pisa.GenerateStorageKey(SlotTotalIncentives)

	// Retrieve the value for the storage key
	val, err := storage.GetStorageEntry(key)
	if err != nil {
		return 0, err
	}

	var totalIncentives uint64
	if err = polo.Depolorize(&totalIncentives, val); err != nil {
		return 0, err
	}

	return totalIncentives, nil
}
