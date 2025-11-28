package compute

import (
	"fmt"
	"math/big"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/state"
)

type AssetEngineImpl struct {
	transition *state.Transition
}

func NewAssetEngine(transition *state.Transition) *AssetEngineImpl {
	return &AssetEngineImpl{transition: transition}
}

func (e *AssetEngineImpl) BalanceOf(address identifiers.Identifier,
	assetID identifiers.AssetID, tokenID common.TokenID,
) (*big.Int, error) {
	object, err := e.transition.GetObject(address)
	if err != nil {
		return nil, err
	}

	return object.BalanceOf(assetID, tokenID)
}

func (e *AssetEngineImpl) Symbol(assetID identifiers.AssetID) (string, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return "", err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return "", err
	}

	return properties.Symbol, nil
}

func (e *AssetEngineImpl) Creator(assetID identifiers.AssetID) (identifiers.Identifier, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return identifiers.Identifier{}, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return identifiers.Nil, err
	}

	return properties.Creator, nil
}

func (e *AssetEngineImpl) Manager(assetID identifiers.AssetID) (identifiers.Identifier, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return identifiers.Identifier{}, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return identifiers.Nil, err
	}

	return properties.Manager, nil
}

func (e *AssetEngineImpl) Decimals(assetID identifiers.AssetID) (uint8, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return 0, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return 0, err
	}

	return properties.Decimals, nil
}

func (e *AssetEngineImpl) MaxSupply(assetID identifiers.AssetID) (*big.Int, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return nil, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return nil, err
	}

	return properties.MaxSupply, nil
}

func (e *AssetEngineImpl) CirculatingSupply(assetID identifiers.AssetID) (*big.Int, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return nil, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return nil, err
	}

	return properties.CirculatingSupply, nil
}

func (e *AssetEngineImpl) LogicID(assetID identifiers.AssetID) (identifiers.LogicID, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return identifiers.Nil, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return identifiers.Nil, err
	}

	return properties.LogicID, nil
}

func (e *AssetEngineImpl) EnableEvents(assetID identifiers.AssetID) (bool, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return false, err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return false, err
	}

	return properties.EnableEvents, nil
}

func (e *AssetEngineImpl) SetStaticMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	key string, val []byte,
) error {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return err
	}

	if properties.Manager != participantID {
		return common.ErrManagerMismatch
	}

	return object.SetAssetMetadata(assetID, true, key, val)
}

func (e *AssetEngineImpl) SetDynamicMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	key string, val []byte,
) error {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return err
	}

	properties, err := object.GetProperties(assetID)
	if err != nil {
		return err
	}

	if properties.Manager != participantID {
		return common.ErrManagerMismatch
	}

	return object.SetAssetMetadata(assetID, false, key, val)
}

func (e *AssetEngineImpl) GetStaticMetaData(assetID identifiers.AssetID, key string) ([]byte, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return nil, err
	}

	return object.GetAssetMetadata(assetID, true, key)
}

func (e *AssetEngineImpl) GetDynamicMetaData(assetID identifiers.AssetID, key string) ([]byte, error) {
	object, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return nil, err
	}

	return object.GetAssetMetadata(assetID, false, key)
}

func (e *AssetEngineImpl) SetStaticTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	tokenID common.TokenID, key string, val []byte,
) error {
	participant, err := e.transition.GetObject(participantID)
	if err != nil {
		return err
	}

	return participant.SetTokenMetadata(assetID, tokenID, true, key, val)
}

func (e *AssetEngineImpl) SetDynamicTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	tokenID common.TokenID, key string, val []byte,
) error {
	participant, err := e.transition.GetObject(participantID)
	if err != nil {
		return err
	}

	return participant.SetTokenMetadata(assetID, tokenID, false, key, val)
}

func (e *AssetEngineImpl) GetStaticTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	tokenID common.TokenID, key string,
) ([]byte, error) {
	participant, err := e.transition.GetObject(participantID)
	if err != nil {
		return nil, err
	}

	return participant.GetTokenMetadata(assetID, tokenID, true, key)
}

func (e *AssetEngineImpl) GetDynamicTokenMetaData(assetID identifiers.AssetID, participantID identifiers.Identifier,
	tokenID common.TokenID, key string,
) ([]byte, error) {
	participant, err := e.transition.GetObject(participantID)
	if err != nil {
		return nil, err
	}

	return participant.GetTokenMetadata(assetID, tokenID, false, key)
}

func (e *AssetEngineImpl) CreateAsset(
	ixHash common.Hash,
	assetID identifiers.AssetID,
	symbol string,
	decimals uint8,
	dimension uint8,
	manager identifiers.Identifier,
	creator identifiers.Identifier,
	maxSupply *big.Int,
	staticMetadata, dynamicMetadata map[string][]byte,
	enableEvents bool,
	logicID identifiers.LogicID,
) (uint64, error) {
	fuel := FuelAssetCreation

	sender, err := e.transition.GetObject(creator)
	if err != nil {
		return fuel, err
	}

	assetacc, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return fuel, err
	}

	descriptor := common.NewAssetDescriptor(
		assetID,
		symbol,
		decimals,
		dimension,
		manager,
		creator,
		maxSupply,
		staticMetadata, dynamicMetadata,
		enableEvents,
		logicID,
	)

	if _, err = createAsset(sender, assetacc, descriptor); err != nil {
		return fuel, err
	}

	if err = addNewAccountsToSargaAccount(e.transition, ixHash, assetacc.Identifier()); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Transfer(
	assetID identifiers.AssetID, tokenID common.TokenID,
	operatorID, benefactorID, beneficiaryID identifiers.Identifier,
	amount *big.Int,
) (uint64, error) {
	fuel := FuelSimpleAssetTransfer

	operator, err := e.transition.GetObject(operatorID)
	if err != nil {
		return fuel, err
	}

	target, err := e.transition.GetObject(beneficiaryID)
	if err != nil {
		return fuel, err
	}

	var sarga *state.Object

	// First try to load a non-auxiliary sarga object
	sarga, err = e.transition.GetObject(common.SargaAccountID)
	if err != nil {
		// Then try to load auxiliary sarga object
		sarga, err = e.transition.GetAuxiliaryObject(common.SargaAccountID)
		if err != nil {
			return fuel, err
		}
	}

	if benefactorID.IsNil() {
		// Validate asset transfer payload
		if err = validateAssetTransfer(operator, target, sarga, assetID, tokenID, amount); err != nil {
			return fuel, err
		}

		// Transfer the asset amount from the operator to target account
		if err = transferAsset(operator, target, assetID, tokenID, amount); err != nil {
			return fuel, err
		}

		return fuel, nil
	}

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	// Validate asset consume payload
	if err = validateAssetConsume(operatorID, target, benefactor, sarga, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	// Transfer the asset amount from the benefactor to target account
	if err = consumeMandate(operatorID, benefactor, target, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Mint(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	senderID, beneficiaryID identifiers.Identifier,
	amount *big.Int,
	staticMetadata map[string][]byte,
) (uint64, error) {
	fuel := FuelAssetMint

	beneficiary, err := e.transition.GetObject(beneficiaryID)
	if err != nil {
		return fuel, err
	}

	assetacc, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return fuel, err
	}

	if err = validateAssetMint(senderID, assetacc, assetID, amount); err != nil {
		return fuel, err
	}

	if err = mintAsset(beneficiary, assetacc, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	for k, v := range staticMetadata {
		if err = e.SetStaticTokenMetaData(assetID, senderID, tokenID, k, v); err != nil {
			return fuel, err
		}
	}

	return fuel, err
}

func (e *AssetEngineImpl) Burn(
	assetID identifiers.AssetID, tokenID common.TokenID, benefactorID identifiers.Identifier, amount *big.Int,
) (uint64, error) {
	fuel := FuelAssetBurn

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	assetacc, err := e.transition.GetObject(assetID.AsIdentifier())
	if err != nil {
		return fuel, err
	}

	if err = validateAssetBurn(benefactor, assetacc, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	if err = burnAsset(benefactor, assetacc, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Approve(assetID identifiers.AssetID, tokenID common.TokenID,
	benefactorID, beneficiaryID identifiers.Identifier, amount *big.Int, expiresAt uint64,
) (uint64, error) {
	fuel := FuelAssetApprove

	fmt.Println("Inside approval")

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	// Validate asset approve payload
	if err = validateAssetApprove(benefactor, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	if err = benefactor.CreateMandate(assetID, tokenID, beneficiaryID, amount, expiresAt); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Revoke(assetID identifiers.AssetID, tokenID common.TokenID,
	benefactorID, beneficiaryID identifiers.Identifier,
) (uint64, error) {
	fuel := FuelAssetRevoke

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	if err = validateAssetRevoke(benefactor, beneficiaryID, assetID, tokenID); err != nil {
		return fuel, err
	}

	if err = revokeAsset(benefactor, beneficiaryID, assetID, tokenID); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Lockup(assetID identifiers.AssetID, tokenID common.TokenID,
	benefactorID, beneficiaryID identifiers.Identifier, amount *big.Int,
) (uint64, error) {
	fuel := FuelAssetLockup

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	// Validate asset lockup payload
	if err = validateAssetLockup(benefactor, beneficiaryID, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	if err = lockupAsset(benefactor, beneficiaryID, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	return fuel, nil
}

func (e *AssetEngineImpl) Release(assetID identifiers.AssetID, tokenID common.TokenID,
	operatorID, benefactorID, beneficiaryID identifiers.Identifier, amount *big.Int,
) (uint64, error) {
	fuel := FuelAssetRelease

	benefactor, err := e.transition.GetObject(benefactorID)
	if err != nil {
		return fuel, err
	}

	beneficiary, err := e.transition.GetObject(beneficiaryID)
	if err != nil {
		return fuel, err
	}

	// Validate asset release payload
	if err = validateAssetRelease(operatorID, benefactor, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	// Transfer the lockup amount from the benefactor to target account
	if err = releaseAsset(operatorID, benefactor, beneficiary, assetID, tokenID, amount); err != nil {
		return fuel, err
	}

	return 0, nil
}
