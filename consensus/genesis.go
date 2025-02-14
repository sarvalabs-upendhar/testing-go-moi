package consensus

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/state"

	"github.com/sarvalabs/go-moi/common"
)

func (k *Engine) SetupGenesis() error {
	transition := make(state.ObjectMap)

	sargaAccount, genesisAccounts, assetAccounts, logics, err := k.parseGenesisFile()
	if err != nil {
		return errors.Wrap(err, "failed to parse genesis file")
	}

	if _, err = k.state.GetAccountMetaInfo(sargaAccount.ID); err == nil {
		return nil
	}

	sargaObject, err := k.setupSargaAccount(sargaAccount, genesisAccounts, assetAccounts, logics)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	transition[sargaObject.Identifier()] = sargaObject

	for _, v := range genesisAccounts {
		if transition[v.ID], err = k.setupNewAccount(v); err != nil {
			return errors.Wrap(err, "failed to setup genesis account")
		}

		accountKeys := make(common.AccountKeys, len(v.Keys))

		for i, key := range v.Keys {
			accountKeys[i] = &common.AccountKey{
				ID:                 uint64(i),
				PublicKey:          key.PublicKey,
				Weight:             key.Weight.ToUint64(),
				SignatureAlgorithm: key.SignatureAlgorithm.ToUint64(),
				Revoked:            false,
				SequenceID:         0,
			}
		}

		transition[v.ID].UpdateKeys(accountKeys)
	}

	if _, err = k.setupGenesisLogics(transition, logics); err != nil {
		return errors.Wrap(err, "failed to setup genesis logic")
	}

	if err = k.setupAssetAccounts(transition, assetAccounts); err != nil {
		return errors.Wrap(err, "failed to setup asset accounts")
	}

	count := len(transition)
	ids := make([]identifiers.Identifier, 0, count)
	stateHashes := make([]common.Hash, 0, count)
	contextHashes := make([]common.Hash, 0, count)

	for _, stateObject := range transition {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		ids = append(ids, stateObject.Identifier())
		stateHashes = append(stateHashes, stateHash)
		contextHashes = append(contextHashes, stateObject.ContextHash())
	}

	if err = k.addGenesisTesseract(ids, stateHashes, contextHashes, transition); err != nil {
		return err
	}

	return nil
}

func createGenesisTesseract(
	ids []identifiers.Identifier,
	stateHashes, contextHashes []common.Hash,
	timestamp uint64, icsSeed, icsProof string,
	transition state.ObjectMap,
) *common.Tesseract {
	var (
		ixHashString = "Genesis"
		participants = make(common.ParticipantsState)
	)

	for i, id := range ids {
		participants[id] = common.State{
			Height:          0,
			TransitiveLink:  common.NilHash,
			PreviousContext: common.NilHash,
			LatestContext:   contextHashes[i],
			StateHash:       stateHashes[i],
			ContextDelta: &common.DeltaGroup{
				ConsensusNodes: transition.GetObject(id).ConsensusNodes(),
			},
		}
	}

	sort.Slice(stateHashes, func(i, j int) bool {
		return stateHashes[i].Hex() < stateHashes[j].Hex()
	})

	for i := 0; i < len(stateHashes); i++ {
		ixHashString += stateHashes[i].Hex()
	}

	interactionsHash := common.GetHash([]byte(ixHashString))

	poxt := common.PoXtData{
		View:     common.GenesisView,
		ICSSeed:  common.HexToHash(icsSeed),
		ICSProof: common.Hex2Bytes(icsProof),
	}

	ts := common.NewTesseract(
		participants,
		interactionsHash,
		common.NilHash,
		big.NewInt(0),
		timestamp,
		0,
		0,
		poxt,
		nil,
		"",
		common.Interactions{},
		nil,
		&common.CommitInfo{
			View: common.GenesisView,
		},
	)

	ts.SetCommitQc(&common.Qc{
		View:   common.GenesisView,
		TSHash: ts.Hash(),
	})

	return ts
}

func (k *Engine) addGenesisTesseract(
	ids []identifiers.Identifier,
	stateHashes, contextHashes []common.Hash,
	transition state.ObjectMap,
) error {
	tesseract := createGenesisTesseract(
		ids,
		stateHashes,
		contextHashes,
		k.cfg.GenesisTimestamp,
		k.cfg.GenesisSeed,
		k.cfg.GenesisProof,
		transition,
	)

	if err := k.lattice.AddTesseract(
		true,
		identifiers.Nil,
		tesseract,
		state.NewTransition(transition, nil),
		true,
	); err != nil {
		return errors.Wrap(err, "error adding genesis tesseract")
	}

	return nil
}

func (k *Engine) setupSargaAccount(
	sarga *common.AccountSetupArgs,
	accounts []common.AccountSetupArgs,
	assets []common.AssetAccountSetupArgs,
	logics []common.LogicSetupArgs,
) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(common.SargaAccountID, common.SargaAccount, true)

	if err := stateObject.CreateContext(sarga.ConsensusNodes); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(common.SargaLogicID); err != nil {
		return nil, errors.Wrap(err, "failed to create storage tree")
	}

	if err := stateObject.AddAccountGenesisInfo(common.SargaAccountID, common.GenesisIxHash); err != nil {
		return nil, err
	}

	for _, account := range accounts {
		// Add account to sarga storage tree
		if err := stateObject.AddAccountGenesisInfo(account.ID, common.GenesisIxHash); err != nil {
			return nil, err
		}
	}

	for _, logic := range logics {
		// Add logic account to sarga
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateLogicIDFromString(logic.Name, 0,
				identifiers.LogicIntrinsic,
				identifiers.LogicExtrinsic, identifiers.Systemic).AsIdentifier(),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	for _, assetAcc := range assets {
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAssetIDFromString(
				assetAcc.AssetInfo.Symbol,
				0,
				uint16(assetAcc.AssetInfo.Standard),
				assetAcc.AssetInfo.AssetDescriptor().Flags()...,
			).AsIdentifier(),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	return stateObject, nil
}

func (k *Engine) setupNewAccount(info common.AccountSetupArgs) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(info.ID, info.AccType, true)

	if err := stateObject.CreateContext(info.ConsensusNodes); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	accountKeys := make(common.AccountKeys, len(info.Keys))

	for i, key := range info.Keys {
		accountKeys[i] = &common.AccountKey{
			ID:                 uint64(i),
			PublicKey:          key.PublicKey.Bytes(),
			Weight:             key.Weight.ToUint64(),
			SignatureAlgorithm: key.SignatureAlgorithm.ToUint64(),
			Revoked:            false,
			SequenceID:         0,
		}
	}

	stateObject.UpdateKeys(accountKeys)

	return stateObject, nil
}

func (k *Engine) setupGenesisLogics(
	transition map[identifiers.Identifier]*state.Object,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicID := common.CreateLogicIDFromString(logic.Name, 0,
			identifiers.Systemic,
			identifiers.LogicIntrinsic,
			identifiers.LogicExtrinsic,
		).AsIdentifier()

		if !common.ContainsID(common.GenesisLogicIDs, logicID) {
			k.logger.Error("Mismatch of logic id", "logic-name", logic.Name, logicID)

			return nil, errors.New("generated id does not exist in predefined logic ids")
		}

		// Create state object for the logic
		logicState := k.state.CreateStateObject(logicID, common.LogicAccount, true)

		// Create a dummy state object for the deployer
		// NOTE: This is a dummy object we create at genesis deployment with the 0x00..00 id
		// to act as a placeholder account for the execution environment's sender state driver.
		deployerState := k.state.CreateStateObject(identifiers.Nil, common.RegularAccount, true)

		consensusNodes := logic.ConsensusNodes

		err := logicState.CreateContext(consensusNodes)
		if err != nil {
			return nil, errors.Wrap(err, "context initiation failed in genesis")
		}

		// Create a new execution context
		ctx := &common.ExecutionContext{
			CtxDelta: nil,
			Cluster:  "genesis",
			Time:     k.cfg.GenesisTimestamp,
		}

		// Create a new IxLogicDeploy interaction with the logic payload
		ix, err := common.NewInteraction(common.IxData{
			Participants: []common.IxParticipant{
				{
					ID: common.SargaAccountID,
				},
			},
			Sender: common.Sender{
				ID: common.SargaAccountID,
			},
			IxOps: []common.IxOpRaw{
				{
					Type: common.IxLogicDeploy,
					Payload: func() []byte {
						payload := &common.LogicPayload{
							Callsite: logic.Callsite,
							Calldata: logic.Calldata,
							Manifest: logic.Manifest.Bytes(),
						}

						encoded, _ := payload.Bytes()

						return encoded
					}(),
				},
			},
			FuelPrice: big.NewInt(0),
		}, nil)
		if err != nil {
			panic(err)
		}

		// Deploy the genesis logic and check for errors
		_, receipt, err := compute.DeployLogic(
			ctx, ix.GetIxOp(0), logicState,
			deployerState, compute.NewEventStream(identifiers.Nil),
		)
		if err != nil {
			k.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "deployment failed for logic")
		}

		if receipt.Error != nil {
			return nil, errors.Errorf("deployment call failed: %v", receipt.Error)
		}

		// Update the dirty objects map with the logic state object
		transition[logicState.Identifier()] = logicState

		// Obtain the logic ID from the call receipt

		k.logger.Info("Deployed genesis contract",
			"logic-name", logic.Name,
			"logic-ID", receipt.LogicID.String(),
		)
	}

	return hashes, nil
}

func (k *Engine) setupAssetAccounts(
	transition map[identifiers.Identifier]*state.Object,
	assetAccs []common.AssetAccountSetupArgs,
) error {
	for _, assetAccount := range assetAccs {
		assetInfo := assetAccount.AssetInfo.AssetDescriptor()
		accID := common.CreateAssetIDFromString(
			assetInfo.Symbol,
			0,
			uint16(assetInfo.Standard),
			assetInfo.Flags()...).AsIdentifier()

		transition[accID] = k.state.CreateStateObject(accID, common.AssetAccount, true)

		err := transition[accID].CreateContext(assetAccount.ConsensusNodes)
		if err != nil {
			return err
		}

		assetID, err := transition[accID].CreateAsset(accID, assetAccount.AssetInfo.AssetDescriptor())
		if err != nil {
			return err
		}

		if assetAccount.AssetInfo.Operator != identifiers.Nil {
			if _, ok := transition[assetAccount.AssetInfo.Operator]; !ok {
				return errors.New("operator account not found")
			}

			if err = transition[assetAccount.AssetInfo.Operator].CreateDeedsEntry(assetID.AsIdentifier()); err != nil {
				return err
			}
		}

		for _, allocation := range assetAccount.AssetInfo.Allocations {
			if _, ok := transition[allocation.ID]; !ok {
				return errors.New("allocation address not found in state objects")
			}

			assetObject := state.NewAssetObject(allocation.Amount.ToInt(), nil)

			if err = transition[allocation.ID].InsertNewAssetObject(assetID, assetObject); err != nil {
				return err
			}
		}
	}

	return nil
}

func (k *Engine) validateAccountKeys(keys []common.KeyArgs) error {
	total := uint64(0)

	for _, key := range keys {
		if key.SignatureAlgorithm.ToUint64() != 0 {
			return common.ErrInvalidSignatureAlgorithm
		}

		total += key.Weight.ToUint64()
	}

	if total < common.MinWeight {
		return common.ErrInvalidWeight
	}

	return nil
}

func (k *Engine) validateAccountCreationInfo(accs ...common.AccountSetupArgs) error {
	for _, acc := range accs {
		if acc.ID == common.SargaAccountID {
			return common.ErrInvalidIdentifier
		}

		// check for address validity
		err := utils.ValidateAccountType(acc.AccType)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("invalid genesis account creation info %s", acc.ID))
		}

		if err := k.validateAccountKeys(acc.Keys); err != nil {
			return errors.Wrap(err, fmt.Sprintf("invalid genesis account creation info %s", acc.ID))
		}
	}

	return nil
}

func (k *Engine) validateSargaAccountCreationInfo(acc common.AccountSetupArgs) error {
	if acc.ID != common.SargaAccountID {
		return common.ErrInvalidIdentifier
	}

	return nil
}

func (k *Engine) validateAssetAccountCreationArgs(assetAccounts ...common.AssetAccountSetupArgs) error {
	for _, acc := range assetAccounts {
		if len(acc.AssetInfo.Allocations) == 0 {
			return errors.New("empty allocations")
		}
	}

	return nil
}

func (k *Engine) validateLogicCreationArgs(logicAccounts ...common.LogicSetupArgs) error {
	for _, acc := range logicAccounts {
		if len(acc.Manifest) == 0 {
			return errors.New("invalid manifest")
		}
	}

	return nil
}

func (k *Engine) parseGenesisFile() (
	*common.AccountSetupArgs,
	[]common.AccountSetupArgs,
	[]common.AssetAccountSetupArgs,
	[]common.LogicSetupArgs,
	error,
) {
	genesisData := new(common.GenesisFile)

	data, err := os.ReadFile(k.cfg.GenesisFilePath)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	err = k.validateSargaAccountCreationInfo(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	err = k.validateAccountCreationInfo(genesisData.Accounts...)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if err = k.validateAssetAccountCreationArgs(genesisData.AssetAccounts...); err != nil {
		return nil, nil, nil, nil, err
	}

	if err = k.validateLogicCreationArgs(genesisData.Logics...); err != nil {
		return nil, nil, nil, nil, err
	}

	return &genesisData.SargaAccount, genesisData.Accounts, genesisData.AssetAccounts, genesisData.Logics, nil
}
