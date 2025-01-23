package guardianregistry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
)

func TestGuardianTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())

	// Run GuardianRegistry suite with Setup deployer
	suite.Run(t, new(GuardianSetupTestSuite))
	// Run GuardianRegistry suite with Import deployer
	suite.Run(t, new(GuardianImportTestSuite))
}

var (
	prefixMOIID = "10"

	MasterAddr  = identifiers.RandomParticipantIDv0().AsIdentifier()
	MasterMOIID = prefixMOIID + strings.TrimPrefix(MasterAddr.Hex(), "0x")

	AdminAddr1 = identifiers.RandomParticipantIDv0().AsIdentifier()
	AdminAddr2 = identifiers.RandomParticipantIDv0().AsIdentifier()

	GuardianKramaID1 = "4a77f591c6aadc9f09c9aafe176186"
	GuardianPubKey1  = []byte("14a359f1659c668f50d3b8e7d861db4e80a3ec307b28d8bc4baf97753e707b0f")

	GuardianKramaID2 = "9f613448c905c459d59e960f7d99b0"
	GuardianPubKey2  = []byte("8aa6527a6bce86eab46ef5b643810872c06720c7528e6d5f840cfe1f013575b9")

	GuardianKramaID3 = "e8032074387c5d76b6d603b5623f8f"
	GuardianPubKey3  = []byte("2a96b8e2940b7956c7409ea755475deb70d72b2a70861f7a981d6dc60ee8beb5")

	GuardianKramaID4 = "5fa6234ba28fb0b360d1ce1c82762b"
	GuardianPubKey4  = []byte("35f7487d1eda34ca571d5197da5d362d5d66406d2ea1332c4e4e35815ecdfa51")

	GuardianKramaID5 = "0a6c0bc78d861f5fa8c674283fa9d6"
	GuardianPubKey5  = []byte("51cae1067d2676501e0be65d8b0b8ceb3c3fad0ae184ee627f7f7b1645ef6662")

	GuardianKramaID6 = "36a04a3a2d3febc5e1fdab8083b64a"
	GuardianPubKey6  = []byte("b6f13b8a99a05faf286522b3adc9e64e97f0f0fad78dff024cc05e23ad7c52f9")
)

type GuardianSetupTestSuite struct {
	GuardianTestSuite
}

func (suite *GuardianSetupTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./guardians.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	consumed, err := suite.Initialise(engineio.PISA, manifest, AdminAddr1)
	suite.Require().NoErrorf(err, "could not read initialise test")
	suite.Require().Equal(uint64(0x2413), consumed)

	inputs := struct {
		Master      Master     `polo:"master"`
		Guardians   []string   `polo:"guardians"`
		PubKeys     [][]byte   `polo:"pubkeys"`
		Admins      [][32]byte `polo:"admins"`
		PreApproved []string   `polo:"preApproved"`
		LimitKYC    uint64     `polo:"limitKYC"`
		LimitKYB    uint64     `polo:"limitKYB"`
		Incentives  []uint64   `polo:"incentives"`
	}{
		Master: Master{
			PubKey: MasterAddr.Bytes(),
			MOIID:  MasterMOIID,
			Wallet: MasterAddr,
		},
		Guardians: []string{
			GuardianKramaID1,
			GuardianKramaID2,
		},
		PubKeys: [][]byte{
			GuardianPubKey1,
			GuardianPubKey2,
		},
		Admins: [][32]byte{
			AdminAddr1,
			AdminAddr2,
		},
		PreApproved: []string{
			GuardianKramaID3,
			GuardianKramaID4,
		},
		LimitKYC:   1,
		LimitKYB:   3,
		Incentives: []uint64{100, 200},
	}

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(inputs, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Deploy("Setup", calldata, nil, nil)

	// Check the setup consistency
	suite.T().Run("CheckSetup", suite.testSetup)
}

type GuardianImportTestSuite struct {
	GuardianTestSuite
}

func (suite *GuardianImportTestSuite) SetupSuite() {
	// Read manifest file
	manifest, err := engineio.NewManifestFromFile("./guardians.yaml")
	suite.Require().NoErrorf(err, "could not read manifest file")

	// Initialise the test suite
	consumed, err := suite.Initialise(engineio.PISA, manifest, AdminAddr1)
	suite.Require().NoErrorf(err, "could not read initialise test")
	suite.Require().Equal(uint64(0x2413), consumed)

	inputs := struct {
		Master          Master     `polo:"master"`
		Admins          [][32]byte `polo:"admins"`
		Guardians       []Guardian `polo:"guardians"`
		Operators       []Operator `polo:"operators"`
		Approved        []string   `polo:"approved"`
		ReferralAddrs   [][32]byte `polo:"referralAddrs"`
		ReferralAmounts []uint64   `polo:"referralAmounts"`
		KnownGs         []string   `polo:"knownGs"`
		KnownOs         []string   `polo:"knownOs"`
		LimitKYC        uint64     `polo:"nodeLimitKYC"`
		LimitKYB        uint64     `polo:"nodeLimitKYB"`
	}{
		Master: Master{
			PubKey: MasterAddr.Bytes(),
			MOIID:  MasterMOIID,
			Wallet: MasterAddr,
		},
		Admins: [][32]byte{
			AdminAddr1,
			AdminAddr2,
		},
		Guardians: []Guardian{
			{
				KramaID:    GuardianKramaID1,
				OperatorID: MasterMOIID,
				Incentive: Incentive{
					Amount: 100,
					Wallet: MasterAddr,
				},
				PublicKey: GuardianPubKey1,
			},
			{
				KramaID:    GuardianKramaID2,
				OperatorID: MasterMOIID,
				Incentive: Incentive{
					Amount: 200,
					Wallet: MasterAddr,
				},
				PublicKey: GuardianPubKey2,
			},
		},
		Operators: []Operator{
			{
				Identifier:   MasterMOIID,
				Verification: VerifyProof{},
				Guardians: []string{
					GuardianKramaID1,
					GuardianKramaID2,
				},
			},
		},
		Approved: []string{
			GuardianKramaID1,
			GuardianKramaID2,
			GuardianKramaID3,
			GuardianKramaID4,
		},
		KnownGs: []string{
			GuardianKramaID1,
			GuardianKramaID2,
		},
		KnownOs: []string{
			MasterMOIID,
		},
		LimitKYC: 1,
		LimitKYB: 3,
	}

	// Serialize the input args into calldata
	calldata, err := polo.PolorizeDocument(inputs, polo.DocStructs())
	require.NoError(suite.T(), err)

	// Deploy the logic to initialise its initial state
	suite.Deploy("Import", calldata, nil, nil)

	// Check the setup consistency
	suite.T().Run("CheckSetup", suite.testSetup)
}

type GuardianTestSuite struct {
	engineio.TestSuite
}

func (suite *GuardianTestSuite) testSetup(_ *testing.T) {
	// Check that there are 2 admin addresses
	keyAdminsLen := pisa.GenerateStorageKey(SlotAdministrators)
	suite.CheckPersistentStorage(keyAdminsLen, uint64(2))

	keyAdmin1 := pisa.GenerateStorageKey(SlotAdministrators, pisa.MakeMapKey(AdminAddr1))
	suite.CheckPersistentStorage(keyAdmin1, true)

	keyAdmin2 := pisa.GenerateStorageKey(SlotAdministrators, pisa.MakeMapKey(AdminAddr2))
	suite.CheckPersistentStorage(keyAdmin2, true)

	// Check that there are 4 approved krama IDs
	// This includes the 2 in the pre-approved and 2 in the master guardians
	keyApprovedLen := pisa.GenerateStorageKey(SlotApproved)
	suite.CheckPersistentStorage(keyApprovedLen, uint64(4))

	// Check if Krama ID added to master guardians was approved
	suite.Invoke(
		"IsApproved",
		suite.DocGen(map[string]any{"kramaID": GuardianKramaID1}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)

	// Check if Krama ID in the pre-approved set is approved
	suite.Invoke(
		"IsApproved",
		suite.DocGen(map[string]any{"kramaID": GuardianKramaID3}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)

	// Check if some other KramaID was not approved
	suite.Invoke(
		"IsApproved",
		suite.DocGen(map[string]any{"kramaID": GuardianKramaID6}),
		suite.DocGen(map[string]any{"ok": false}),
		nil,
	)

	// Check if the master MOI ID was marked as verified
	suite.Invoke(
		"IsVerified",
		suite.DocGen(map[string]any{"moiID": MasterMOIID}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)

	// Check if some other MOI ID was not verified
	suite.Invoke(
		"IsVerified",
		suite.DocGen(map[string]any{"moiID": identifiers.RandomParticipantIDv0().AsIdentifier().Hex()}),
		suite.DocGen(map[string]any{"ok": false}),
		nil,
	)

	// Check that there is 1 known operators
	// This is the given master operator
	keyKnownOperatorsLen := pisa.GenerateStorageKey(SlotKnownOperators)
	suite.CheckPersistentStorage(keyKnownOperatorsLen, uint64(1))

	// Check that master MOI ID is at known operators index 0
	keyKnownOperators0 := pisa.GenerateStorageKey(SlotKnownOperators, pisa.ArrIdx(0))
	suite.CheckPersistentStorage(keyKnownOperators0, MasterMOIID)

	// Check that there are 2 known guardians
	// These are the given master guardians
	keyKnownGuardiansLen := pisa.GenerateStorageKey(SlotKnownGuardians)
	suite.CheckPersistentStorage(keyKnownGuardiansLen, uint64(2))

	// Check that krama ID 1 is at known guardians index 0
	keyKnownGuardian0 := pisa.GenerateStorageKey(SlotKnownGuardians, pisa.ArrIdx(0))
	suite.CheckPersistentStorage(keyKnownGuardian0, GuardianKramaID1)

	// Check that krama ID 2 is at known guardians index 1
	keyKnownGuardian1 := pisa.GenerateStorageKey(SlotKnownGuardians, pisa.ArrIdx(1))
	suite.CheckPersistentStorage(keyKnownGuardian1, GuardianKramaID2)

	// Check the master MOI ID
	keyMasterOpMOIID := pisa.GenerateStorageKey(SlotMasterOperator, pisa.ClsFld(0))
	suite.CheckPersistentStorage(keyMasterOpMOIID, MasterMOIID)

	// Check the master wallet address
	keyMasterOpWallet := pisa.GenerateStorageKey(SlotMasterOperator, pisa.ClsFld(1))
	suite.CheckPersistentStorage(keyMasterOpWallet, MasterAddr)

	// Check the master public key
	keyMasterOpPubKey := pisa.GenerateStorageKey(SlotMasterOperator, pisa.ClsFld(1))
	suite.CheckPersistentStorage(keyMasterOpPubKey, MasterAddr.Bytes())

	// Check the public key of the guardian 1
	keyGuardian1PubKey := pisa.GenerateStorageKey(SlotGuardians, pisa.MakeMapKey(GuardianKramaID1), pisa.ClsFld(3))
	suite.CheckPersistentStorage(keyGuardian1PubKey, GuardianPubKey1)
}

func (suite *GuardianTestSuite) TestApprovals() {
	// Approve 2 new Krama IDs
	suite.Invoke(
		"Approve", suite.DocGen(map[string]any{
			"kramaIDs": []string{
				GuardianKramaID5,
				GuardianKramaID6,
			},
		}),
		nil, nil,
	)

	// Check if KramaID 5 was approved successfully
	suite.Invoke(
		"IsApproved",
		suite.DocGen(map[string]any{"kramaID": GuardianKramaID5}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)

	// Check if KramaID 6 was approved successfully
	suite.Invoke(
		"IsApproved",
		suite.DocGen(map[string]any{"kramaID": GuardianKramaID6}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)
}

func (suite *GuardianTestSuite) TestRegistration() {
	// Register a new operator (master)
	// This must fail as operator already exists
	suite.Invoke(
		"RegisterOperator",
		suite.DocGen(map[string]any{
			"moiID":        MasterMOIID,
			"verification": VerifyProof{},
		}),
		nil, pisa.NewError(
			"string", "operator already exists", true,
			[]string{
				"runtime.root()",
				"routine.RegisterOperator() [0x24] ... [0x14: REVERT 0x2]",
			},
		),
	)

	// Create a new address for the operator
	newOp := identifiers.RandomParticipantIDv0().AsIdentifier()
	newOpID := prefixMOIID + strings.TrimPrefix(newOp.Hex(), "0x")

	// Register a new operator
	suite.Invoke(
		"RegisterOperator",
		suite.DocGen(map[string]any{
			"moiID":        newOpID,
			"verification": VerifyProof{"kyc", []byte{}},
		}),
		nil, nil,
	)

	// Check that known operators list has grown by 1
	keyKnownOperatorsLen := pisa.GenerateStorageKey(SlotKnownOperators)
	suite.CheckPersistentStorage(keyKnownOperatorsLen, uint64(2))

	// Check that operators map has grown by 1
	keyOperatorsLen := pisa.GenerateStorageKey(SlotOperators)
	suite.CheckPersistentStorage(keyOperatorsLen, uint64(2))

	// Check if new operator MOI ID is verified
	suite.Invoke(
		"IsVerified",
		suite.DocGen(map[string]any{"moiID": newOpID}),
		suite.DocGen(map[string]any{"ok": true}),
		nil,
	)

	// Attempt to register an already existing guardian
	// SenderID must be the address of the operator
	// This should cause an exception
	suite.Invoke(
		"RegisterGuardian",
		suite.DocGen(map[string]any{
			"guardian": Guardian{
				KramaID:    GuardianKramaID2,
				OperatorID: newOpID,
				Incentive:  Incentive{},
				PublicKey:  nil,
				ExtraData:  nil,
			},
		}),
		nil, pisa.NewError(
			"string", "guardian already exists", false,
			[]string{
				"runtime.root()",
				"routine.RegisterGuardian() [0x25] ... [0x3c: THROW 0x5]",
			},
		),
		engineio.UseSender(newOp),
	)

	// Attempt to register a guardian that is not approved
	// SenderID must be the address of the operator
	// This should cause an exception
	suite.Invoke(
		"RegisterGuardian",
		suite.DocGen(map[string]any{
			"guardian": Guardian{
				KramaID:    GuardianKramaID6,
				OperatorID: newOpID,
				Incentive:  Incentive{},
				PublicKey:  nil,
				ExtraData:  nil,
			},
		}),
		nil, pisa.NewError(
			"string", "guardian krama id is not approved", false,
			[]string{
				"runtime.root()",
				"routine.RegisterGuardian() [0x25] ... [0x4a: CALLR 0x4 0x5 0x4]",
				"routine.CanRegisterGuardian() [0x26] ... [0x11: THROW 0x1]",
			},
		),
		engineio.UseSender(newOp),
	)

	// Register an approved guardian
	// SenderID must be the address of the operator
	// This should work as expected
	suite.Invoke(
		"RegisterGuardian",
		suite.DocGen(map[string]any{
			"guardian": Guardian{
				KramaID:    GuardianKramaID3,
				OperatorID: newOpID,
				Incentive: Incentive{
					Wallet: identifiers.RandomParticipantIDv0().AsIdentifier(),
				},
			},
		}),
		nil, nil,
		engineio.UseSender(newOp),
	)

	// Check that known guardians list has grown by 1
	keyKnownGuardiansLen := pisa.GenerateStorageKey(SlotKnownGuardians)
	suite.CheckPersistentStorage(keyKnownGuardiansLen, uint64(3))

	// Check that operator guardians list has grown by 1
	keyOperatorGuardiansLen := pisa.GenerateStorageKey(SlotOperators, pisa.MakeMapKey(newOpID), pisa.ClsFld(2))
	suite.CheckPersistentStorage(keyOperatorGuardiansLen, uint64(1))

	// Check that krama ID 3 is at the 0th index of the operator's guardians
	keyOpGuardian0 := pisa.GenerateStorageKey(SlotOperators, pisa.MakeMapKey(newOpID), pisa.ClsFld(2), pisa.ArrIdx(0))
	suite.CheckPersistentStorage(keyOpGuardian0, GuardianKramaID3)
}

func (suite *GuardianTestSuite) TestIncentivisation() {
	suite.T().Run("WithoutReferral", func(t *testing.T) {
		// Get the current incentives of krama ID 1. Must be 0
		suite.Invoke(
			"GetIncentives",
			suite.DocGen(map[string]any{"kramaID": GuardianKramaID1}),
			suite.DocGen(map[string]any{"incentive": uint64(100)}),
			nil,
		)

		// Attempt to add incentives.
		// Use mismatched array lengths and expect exception
		suite.Invoke(
			"AddIncentives",
			suite.DocGen(map[string]any{
				"incentiveIDs":     []string{GuardianKramaID1},
				"incentiveAmounts": []uint64{100, 200},
			}),
			nil, pisa.NewError(
				"string", "invalid incentive inputs: mismatched size", false,
				[]string{
					"runtime.root()",
					"routine.AddIncentives() [0x29] ... [0xc: THROW 0x1]",
				},
			),
		)

		// Attempt to add incentives to a guardian who does not exist
		suite.Invoke(
			"AddIncentives",
			suite.DocGen(map[string]any{
				"incentiveIDs":     []string{GuardianKramaID6},
				"incentiveAmounts": []uint64{100},
			}),
			nil, pisa.NewError("string", "guardian does not exist", true,
				[]string{
					"runtime.root()",
					"routine.AddIncentives() [0x29] ... [0x27: REVERT 0x9]",
				},
			),
		)

		// Add incentives for an existing krama ID with valid inputs
		suite.Invoke(
			"AddIncentives",
			suite.DocGen(map[string]any{
				"incentiveIDs":     []string{GuardianKramaID1},
				"incentiveAmounts": []uint64{100},
			}),
			nil, nil,
		)

		// Check the incentive for the krama ID
		keyGuardian1Incentive := pisa.GenerateStorageKey(
			SlotGuardians,
			pisa.MakeMapKey(GuardianKramaID1),
			pisa.ClsFld(2), pisa.ClsFld(0),
		)
		suite.CheckPersistentStorage(keyGuardian1Incentive, 200)
	})

	suite.T().Run("WithReferral", func(t *testing.T) {
		referralWallet := identifiers.RandomParticipantIDv0().AsIdentifier()

		// Register an approved guardian
		// SenderID must be the address of the operator
		// This should work as expected
		suite.Invoke(
			"RegisterGuardian",
			suite.DocGen(map[string]any{
				"guardian": Guardian{
					KramaID:    GuardianKramaID3,
					OperatorID: MasterMOIID,
					Incentive: Incentive{
						ReferralPercent: 50,
						ReferralWallet:  referralWallet,
						Wallet:          identifiers.RandomParticipantIDv0().AsIdentifier(),
					},
				},
			}),
			nil, nil,
			engineio.UseSender(MasterAddr),
		)

		suite.Invoke(
			"AddIncentives",
			suite.DocGen(map[string]any{
				"incentiveIDs":     []string{GuardianKramaID3},
				"incentiveAmounts": []uint64{100},
			}),
			nil, nil,
		)

		// Check the incentive for the krama ID
		keyGuardian1Incentive := pisa.GenerateStorageKey(
			SlotGuardians,
			pisa.MakeMapKey(GuardianKramaID3),
			pisa.ClsFld(2), pisa.ClsFld(0),
		)
		suite.CheckPersistentStorage(keyGuardian1Incentive, 50)

		// Check that the referral amount was added
		keyReferralAmount := pisa.GenerateStorageKey(SlotReferralRewards, pisa.MakeMapKey(referralWallet))
		suite.CheckPersistentStorage(keyReferralAmount, 50)
	})
}

func (suite *GuardianTestSuite) TestChangeNodeLimit() {
	suite.T().Run("KYC", func(t *testing.T) {
		// Generate storage keys for kyc node limit
		keyLimitKYC := pisa.GenerateStorageKey(SlotNodeLimitKYC)
		// Check the initial state of the node limit
		suite.CheckPersistentStorage(keyLimitKYC, uint64(1))

		// Change the KYC limit to 5 and check storage
		suite.Invoke(
			"ChangeNodeLimit", suite.DocGen(map[string]any{
				"kind":    "kyc",
				"updated": 5,
			}),
			nil, nil,
		)
		suite.CheckPersistentStorage(keyLimitKYC, uint64(5))

		// Attempt change the KYC limit to 3
		// This should cause an exception and the storage must be unchanged
		suite.Invoke(
			"ChangeNodeLimit", suite.DocGen(map[string]any{
				"kind":    "kyc",
				"updated": 3,
			}),
			nil, pisa.NewError(
				"string", "updated limit cannot be be less than existing", true,
				[]string{
					"runtime.root()",
					"routine.ChangeNodeLimit() [0x2a] ... [0x13: REVERT 0x5]",
				},
			),
		)
		suite.CheckPersistentStorage(keyLimitKYC, uint64(5))
	})

	suite.T().Run("KYB", func(t *testing.T) {
		// Generate storage keys for kyb node limit
		keyLimitKYB := pisa.GenerateStorageKey(SlotNodeLimitKYB)
		// Check the initial state of the node limit
		suite.CheckPersistentStorage(keyLimitKYB, uint64(3))

		// Change the KYB limit to 7 and check storage
		suite.Invoke(
			"ChangeNodeLimit", suite.DocGen(map[string]any{
				"kind":    "kyb",
				"updated": 7,
			}),
			nil, nil,
		)
		suite.CheckPersistentStorage(keyLimitKYB, uint64(7))

		// Attempt change the KYB limit to 3
		// This should cause an exception and the storage must be unchanged
		suite.Invoke(
			"ChangeNodeLimit", suite.DocGen(map[string]any{
				"kind":    "kyb",
				"updated": 3,
			}),
			nil, pisa.NewError(
				"string", "updated limit cannot be be less than existing", true,
				[]string{
					"runtime.root()",
					"routine.ChangeNodeLimit() [0x2a] ... [0x2a: REVERT 0x4]",
				},
			),
		)
		suite.CheckPersistentStorage(keyLimitKYB, uint64(7))
	})
}
