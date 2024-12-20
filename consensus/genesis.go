package consensus

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
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

	if _, err = k.state.GetAccountMetaInfo(sargaAccount.Address); err == nil {
		return nil
	}

	sargaObject, err := k.setupSargaAccount(sargaAccount, genesisAccounts, assetAccounts, logics)
	if err != nil {
		return errors.Wrap(err, "failed to setup sarga account")
	}

	transition[sargaObject.Address()] = sargaObject

	for _, v := range genesisAccounts {
		if transition[v.Address], err = k.setupNewAccount(v); err != nil {
			return errors.Wrap(err, "failed to setup genesis account")
		}
	}

	if _, err = k.setupGenesisLogics(transition, logics); err != nil {
		return errors.Wrap(err, "failed to setup genesis logic")
	}

	if err = k.setupAssetAccounts(transition, assetAccounts); err != nil {
		return errors.Wrap(err, "failed to setup asset accounts")
	}

	count := len(transition)
	addresses := make([]identifiers.Address, 0, count)
	stateHashes := make([]common.Hash, 0, count)
	contextHashes := make([]common.Hash, 0, count)

	for _, stateObject := range transition {
		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		addresses = append(addresses, stateObject.Address())
		stateHashes = append(stateHashes, stateHash)
		contextHashes = append(contextHashes, stateObject.ContextHash())
	}

	if err = k.addGenesisTesseract(addresses, stateHashes, contextHashes, transition); err != nil {
		return err
	}

	return nil
}

func createGenesisTesseract(
	addresses []identifiers.Address,
	stateHashes, contextHashes []common.Hash,
	timestamp uint64, icsSeed, icsProof string,
	transition state.ObjectMap,
) *common.Tesseract {
	var (
		ixHashString = "Genesis"
		participants = make(common.ParticipantsState)
	)

	for i, addr := range addresses {
		participants[addr] = common.State{
			Height:          0,
			TransitiveLink:  common.NilHash,
			PreviousContext: common.NilHash,
			LatestContext:   contextHashes[i],
			StateHash:       stateHashes[i],
			ContextDelta: &common.DeltaGroup{
				BehaviouralNodes: transition.GetObject(addr).BehaviourContextObj().Ids,
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
	addresses []identifiers.Address,
	stateHashes, contextHashes []common.Hash,
	transition state.ObjectMap,
) error {
	tesseract := createGenesisTesseract(
		addresses,
		stateHashes,
		contextHashes,
		k.cfg.GenesisTimestamp,
		k.cfg.GenesisSeed,
		k.cfg.GenesisProof,
		transition,
	)

	if err := k.lattice.AddTesseract(
		true,
		identifiers.NilAddress,
		tesseract,
		state.NewTransition(transition),
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
	stateObject := k.state.CreateStateObject(common.SargaAddress, common.SargaAccount, true)

	if _, err := stateObject.CreateContext(sarga.BehaviouralContext, sarga.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	if err := stateObject.CreateStorageTreeForLogic(common.SargaLogicID); err != nil {
		return nil, errors.Wrap(err, "failed to create storage tree")
	}

	if err := stateObject.AddAccountGenesisInfo(common.SargaAddress, common.GenesisIxHash); err != nil {
		return nil, err
	}

	for _, account := range accounts {
		// Add account to sarga storage tree
		if err := stateObject.AddAccountGenesisInfo(account.Address, common.GenesisIxHash); err != nil {
			return nil, err
		}
	}

	for _, logic := range logics {
		// Add logic account to sarga
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(logic.Name),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	for _, assetAcc := range assets {
		if err := stateObject.AddAccountGenesisInfo(
			common.CreateAddressFromString(assetAcc.AssetInfo.Symbol),
			common.GenesisIxHash,
		); err != nil {
			return nil, err
		}
	}

	return stateObject, nil
}

func (k *Engine) setupNewAccount(info common.AccountSetupArgs) (*state.Object, error) {
	stateObject := k.state.CreateStateObject(info.Address, info.AccType, true)

	if _, err := stateObject.CreateContext(info.BehaviouralContext, info.RandomContext); err != nil {
		return nil, errors.Wrap(err, "context initiation failed in genesis")
	}

	return stateObject, nil
}

func (k *Engine) setupGenesisLogics(
	transition map[identifiers.Address]*state.Object,
	logics []common.LogicSetupArgs,
) ([]common.Hash, error) {
	hashes := make([]common.Hash, len(logics))

	for _, logic := range logics {
		logicAddr := common.CreateAddressFromString(logic.Name)

		if !common.ContainsAddress(common.GenesisLogicAddrs, logicAddr) {
			k.logger.Error("Mismatch of contract address", "logic-name", logic.Name)

			return nil, errors.New("generated address does not exist in predefined contract address")
		}

		// Create state object for the logic
		logicState := k.state.CreateStateObject(logicAddr, common.LogicAccount, true)

		// Create a dummy state object for the deployer
		// NOTE: This is a dummy object we create at genesis deployment with the 0x00..00 address
		// to act as a placeholder account for the execution environment's sender state driver.
		deployerState := k.state.CreateStateObject(identifiers.NilAddress, common.RegularAccount, true)

		behaviouralCtx := logic.BehaviouralContext
		randomCtx := logic.RandomContext

		_, err := logicState.CreateContext(behaviouralCtx, randomCtx)
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
					Address: common.SargaAddress,
				},
			},
			Sender: common.SargaAddress,
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
			deployerState, compute.NewEventStream(""),
		)
		if err != nil {
			k.logger.Error("Unable to deploy logic for", "logic-name", logic.Name)

			return nil, errors.Wrap(err, "deployment failed for logic")
		}

		if receipt.Error != nil {
			return nil, errors.Errorf("deployment call failed: %v", receipt.Error)
		}

		// Update the dirty objects map with the logic state object
		transition[logicState.Address()] = logicState

		// Obtain the logic ID from the call receipt
		logicID := receipt.LogicID
		k.logger.Info("Deployed genesis contract",
			"logic-name", logic.Name,
			"logic-ID", logicID.String(),
		)
	}

	return hashes, nil
}

func (k *Engine) setupAssetAccounts(
	transition map[identifiers.Address]*state.Object,
	assetAccs []common.AssetAccountSetupArgs,
) error {
	for _, assetAccount := range assetAccs {
		accAddress := common.CreateAddressFromString(assetAccount.AssetInfo.Symbol)

		transition[accAddress] = k.state.CreateStateObject(accAddress, common.AssetAccount, true)

		_, err := transition[accAddress].CreateContext(assetAccount.BehaviouralContext, assetAccount.RandomContext)
		if err != nil {
			return err
		}

		assetID, err := transition[accAddress].CreateAsset(accAddress, assetAccount.AssetInfo.AssetDescriptor())
		if err != nil {
			return err
		}

		if assetAccount.AssetInfo.Operator != identifiers.NilAddress {
			if _, ok := transition[assetAccount.AssetInfo.Operator]; !ok {
				return errors.New("operator account not found")
			}

			if err = transition[assetAccount.AssetInfo.Operator].CreateDeedsEntry(string(assetID)); err != nil {
				return err
			}
		}

		for _, allocation := range assetAccount.AssetInfo.Allocations {
			if _, ok := transition[allocation.Address]; !ok {
				return errors.New("allocation address not found in state objects")
			}

			assetObject := state.NewAssetObject(allocation.Amount.ToInt(), nil)

			if err = transition[allocation.Address].InsertNewAssetObject(assetID, assetObject); err != nil {
				return err
			}
		}
	}

	return nil
}

func (k *Engine) validateAccountCreationInfo(accs ...common.AccountSetupArgs) error {
	for _, acc := range accs {
		if acc.Address == common.SargaAddress {
			return common.ErrInvalidAddress
		}
		// check for address validity
		err := utils.ValidateAccountType(acc.AccType)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("invalid genesis account creation info %s", acc.Address))
		}
	}

	return nil
}

func (k *Engine) validateSargaAccountCreationInfo(acc common.AccountSetupArgs) error {
	if acc.Address != common.SargaAddress {
		return common.ErrInvalidAddress
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
