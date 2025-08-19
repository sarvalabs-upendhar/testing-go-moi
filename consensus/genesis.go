package consensus

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/state"

	"github.com/sarvalabs/go-moi/common"
)

func (k *Engine) SetupGenesis() error {
	objs := make(state.ObjectMap)

	sargaAccount, systemAccount, genesisAccounts, assetAccounts, logics, err := k.parseGenesisFile()
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

	objs[sargaObject.Identifier()] = sargaObject

	for _, v := range genesisAccounts {
		if objs[v.ID], err = k.setupNewAccount(v); err != nil {
			return errors.Wrap(err, "failed to setup genesis account")
		}

		objs[v.ID].UpdateKeys(getKeys(v.Keys))
	}

	if _, err = k.setupGenesisLogics(objs, logics); err != nil {
		return errors.Wrap(err, "failed to setup genesis logic")
	}

	if err = k.setupAssetAccounts(objs, assetAccounts); err != nil {
		return errors.Wrap(err, "failed to setup asset accounts")
	}

	systemObject, err := k.setupSystemAccount(systemAccount)
	if err != nil {
		return errors.Wrap(err, "failed to setup system account")
	}

	transition := state.NewTransition(systemObject, objs, nil)

	if err = k.updateValidatorStakes(transition); err != nil {
		return errors.Wrap(err, "failed to update validator stakes")
	}

	commitHashes, err := transition.Commit()
	if err != nil {
		return err
	}

	if err = k.addGenesisTesseract(commitHashes, transition); err != nil {
		return err
	}

	return nil
}

func createGenesisTesseract(
	commitHashes common.AccountStateHashes,
	timestamp uint64, icsSeed, icsProof string,
	transition *state.Transition,
) *common.Tesseract {
	var (
		ixHashString = "Genesis_ixns"
		participants = make(common.ParticipantsState)
	)

	for id, info := range commitHashes {
		participants[id] = common.State{
			Height:         0,
			TransitiveLink: common.NilHash,
			LockedContext:  common.NilHash,
			StateHash:      info.StateHash,
			ContextDelta: &common.DeltaGroup{
				ConsensusNodes: transition.GetConsensusNodes(id),
			},
		}
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

func (k *Engine) addGenesisTesseract(commitHashes common.AccountStateHashes, transition *state.Transition) error {
	tesseract := createGenesisTesseract(
		commitHashes,
		k.cfg.GenesisTimestamp,
		k.cfg.GenesisSeed,
		k.cfg.GenesisProof,
		transition,
	)

	if err := k.lattice.AddTesseract(
		true,
		identifiers.Nil,
		tesseract,
		transition,
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

func (k *Engine) setupSystemAccount(systemAcc *common.SystemAccountSetupArgs) (*state.SystemObject, error) {
	systemObject := k.state.CreateSystemObject(systemAcc.ID)

	if err := systemObject.CreateContext(systemAcc.ConsensusNodes); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := systemObject.CreateStorageTreeForLogic(common.SystemLogicID); err != nil {
		return nil, errors.Wrap(err, "failed to create storage tree")
	}

	if err := systemObject.SetGenesisTime(time.Unix(0, int64(k.cfg.GenesisTimestamp))); err != nil {
		return nil, errors.Wrap(err, "failed to set genesis time")
	}

	if err := systemObject.SetValidators(systemAcc.Validators); err != nil {
		return nil, errors.Wrap(err, "failed to set validators")
	}

	return systemObject, nil
}

func (k *Engine) updateValidatorStakes(transition *state.Transition) error {
	systemObj := transition.GetSystemObject()
	if systemObj == nil {
		return errors.New("system object is nil")
	}

	for index, val := range systemObj.Validators() {
		obj := transition.GetObject(val.WalletAddress)
		if obj == nil {
			return errors.Errorf("no object for validator %d", index)
		}

		if err := obj.CreateLockup(
			common.KMOITokenAssetID,
			common.SystemAccountID,
			val.ActiveStake,
		); err != nil {
			return errors.Wrap(err, "failed to create lockup")
		}
	}

	return nil
}

func getKeys(keys []common.KeyArgs) common.AccountKeys {
	accountKeys := make(common.AccountKeys, len(keys))

	for i, key := range keys {
		accountKeys[i] = &common.AccountKey{
			ID:                 uint64(i),
			PublicKey:          key.PublicKey.Bytes(),
			Weight:             key.Weight.ToUint64(),
			SignatureAlgorithm: key.SignatureAlgorithm.ToUint64(),
			Revoked:            false,
			SequenceID:         0,
		}
	}

	return accountKeys
}

func (k *Engine) setupNewAccount(info common.AccountSetupArgs) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(info.ID, info.AccType, true)

	if err := stateObject.CreateContext(info.ConsensusNodes); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	stateObject.UpdateKeys(getKeys(info.Keys))

	return stateObject, nil
}

func (k *Engine) setupGenesisLogics(
	objectMap state.ObjectMap,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicID := common.CreateLogicIDFromString(logic.Name, 0,
			identifiers.Systemic,
			identifiers.LogicIntrinsic,
			identifiers.LogicExtrinsic,
		).AsIdentifier()

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
		execCtx := &common.ExecutionContext{
			CtxDelta: nil,
			Cluster:  "genesis",
			Time:     k.cfg.GenesisTimestamp,
		}

		// Create a new IxLogicDeploy interaction with the logic payload
		ix, err := common.NewInteraction(common.IxData{
			Participants: []common.IxParticipant{
				{
					ID: deployerState.Identifier(),
				},
			},
			Sender: common.Sender{
				ID: deployerState.Identifier(),
			},
			IxOps: []common.IxOpRaw{
				{
					Type: common.IxLogicDeploy,
					Payload: func() []byte {
						payload := &common.LogicPayload{
							Callsite: logic.Callsite,
							Calldata: logic.Calldata.Bytes(),
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

		engine, ok := engineio.FetchEngine(engineio.PISA)
		if !ok {
			return nil, errors.New("failed to fetch engine")
		}

		ctx := &engineio.RuntimeContext{
			ClusterContext: execCtx,
			Runtime:        engine.Runtime(execCtx.Time),
		}

		err = compute.AddActorsToRuntime(ix, ctx.Runtime, state.NewTransition(nil, objectMap, nil))
		if err != nil {
			return nil, err
		}

		// Deploy the genesis logic and check for errors
		_, receipt, _, err := compute.DeployLogic(ctx,
			ix.GetIxOp(0),
			logicState,
			deployerState,
			compute.NewFuelTank(math.MaxUint64, math.MaxUint64),
		)
		if err != nil {
			k.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "deployment failed for logic")
		}

		if receipt.Error != nil {
			return nil, errors.Errorf("deployment call failed: %v", receipt.Error)
		}

		// Update the dirty objects map with the logic state object
		objectMap[logicState.Identifier()] = logicState

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

func (k *Engine) validateSargaAccountCreationArgs(acc common.AccountSetupArgs) error {
	if acc.ID != common.SargaAccountID {
		return common.ErrInvalidIdentifier
	}

	return nil
}

func (k *Engine) validateSystemAccountCreationArgs(acc common.SystemAccountSetupArgs) error {
	if acc.ID != common.SystemAccountID {
		return errors.New("system account id mismatch")
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
	*common.SystemAccountSetupArgs,
	[]common.AccountSetupArgs,
	[]common.AssetAccountSetupArgs,
	[]common.LogicSetupArgs,
	error,
) {
	genesisData := new(common.GenesisFile)

	data, err := os.ReadFile(k.cfg.GenesisFilePath)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Wrap(err, "failed to open genesis file")
	}

	if err = json.Unmarshal(data, genesisData); err != nil {
		return nil, nil, nil, nil, nil, errors.Wrap(err, "failed to parse genesis file")
	}

	err = k.validateSargaAccountCreationArgs(genesisData.SargaAccount)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Wrap(err, "invalid sarga account info")
	}

	err = k.validateSystemAccountCreationArgs(genesisData.SystemAccount)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Wrap(err, "invalid system account info")
	}

	err = k.validateAccountCreationInfo(genesisData.Accounts...)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = k.validateAssetAccountCreationArgs(genesisData.AssetAccounts...); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = k.validateLogicCreationArgs(genesisData.Logics...); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return &genesisData.SargaAccount, &genesisData.SystemAccount, genesisData.Accounts,
		genesisData.AssetAccounts, genesisData.Logics, nil
}
