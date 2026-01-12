package pisa

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	pisa "github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/metering"
	pstorage "github.com/sarvalabs/go-pisa/storage"
)

type RuntimeWrapper struct {
	rn *pisa.Runtime
	as engineio.AssetEngine
}

type Pisa struct{}

func NewEngine() *Pisa {
	return &Pisa{}
}

func (p *Pisa) GenerateManifestElement(kind engineio.ElementKind) (any, bool) {
	element, ok := ElementMetadata[kind]
	if !ok {
		return nil, false
	}

	return element.generator(), true
}

func (p *Pisa) Kind() engineio.EngineKind {
	return engineio.PISA
}

func (p *Pisa) Version() string { return pisa.Version }

func (p *Pisa) Runtime(timestamp uint64) engineio.Runtime {
	r := NewRuntimeWrapper(
		pisa.NewRuntime(
			timestamp,
			0, 0,
			pisa.WithCryptography(Crypto(0))))

	return r
}

func (p *Pisa) CompileManifest(
	manifestKind engineio.ManifestKind,
	logicID identifiers.Identifier,
	manifest engineio.Manifest,
	fuel engineio.FuelGauge,
) (
	[]byte,
	*engineio.FuelGauge,
	map[string]struct{},
	error,
) {
	// Check that the Manifest Instance is PISA
	if manifest.Engine().Kind != engineio.PISA {
		return nil, fuel.Consumed(fuel), nil, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a new manifest compiler
	compiler := NewManifestCompiler(manifestKind, logicID, fuel, manifest)
	// Compile the manifest
	descriptor, err := compiler.CompileArtifact()
	if err != nil {
		return nil, fuel.Consumed(compiler.fuel), nil, errors.Wrap(err, "compile error")
	}

	return descriptor, fuel.Consumed(compiler.fuel), compiler.deployerCallsite, nil
}

func NewRuntimeWrapper(rn *pisa.Runtime) *RuntimeWrapper {
	return &RuntimeWrapper{rn: rn}
}

func (rw *RuntimeWrapper) BindAssetEngine(ae engineio.AssetEngine) {
	rw.as = ae
	rw.rn.BindAssetEngine(&AssetEngineWrapper{ae: ae})
}

func (rw *RuntimeWrapper) SpawnLogic(
	logicID [32]byte,
	artifact []byte,
	storage engineio.Storage,
	params map[string][]byte,
) error {
	ops := make([]pisa.ActorOption, 0)
	for k, v := range params {
		ops = append(ops, pisa.SetActorParameter(k, v))
	}

	art, err := pisa.NewBytecodeArtifact(artifact)
	if err != nil {
		return nil
	}

	return rw.rn.Spawn(art, logicID, storage, nil, ops...)
}

func (rw *RuntimeWrapper) CreateAsset(
	ixHash common.Hash,
	assetID identifiers.AssetID,
	symbol string, decimals uint8, dimension uint8,
	manager, creator identifiers.Identifier,
	maxSupply *big.Int,
	staticMetadata, dynamicMetadata map[string][]byte,
	enableEvents bool,
	logicID identifiers.LogicID,
	access map[[32]byte]int,
) (uint64, error) {
	return rw.as.CreateAsset(
		ixHash, assetID, symbol,
		decimals, dimension, manager, creator, maxSupply, staticMetadata, dynamicMetadata, enableEvents, logicID, access)
}

func (rw *RuntimeWrapper) ActorExists(logicID [32]byte) bool {
	return rw.rn.ActorExists(logicID)
}

func (rw *RuntimeWrapper) CreateActor(id [32]byte, storage engineio.Storage, params map[string][]byte) error {
	ops := make([]pisa.ActorOption, 0)
	for k, v := range params {
		ops = append(ops, pisa.SetActorParameter(k, v))
	}

	return rw.rn.CreateActor(id, storage, nil, ops...)
}

func (rw *RuntimeWrapper) Call(
	logicID [32]byte,
	action engineio.Action,
	transition engineio.Transition,
	limit *engineio.FuelGauge,
) *engineio.CallResult {
	result := rw.rn.Call(
		logicID,
		action,
		NewTransitionWrapper(transition),
		metering.SetEffortLimit(metering.ComputeEffort(limit.Compute)),
		metering.SetVolumeLimit(metering.StorageVolume(limit.Storage)),
	)

	return callResult(result)
}

func callResult(result *pisa.CallResult) *engineio.CallResult {
	if result == nil {
		return nil
	}

	logs := make([]common.Log, 0)

	for index, log := range result.Log() {
		logs = append(logs, common.Log{
			LogicID: log.LogicID,
			ID:      log.ActorID,
			Topics:  make([]common.Hash, 0, len(log.Topics)),
			Data:    log.Values.Bytes(),
		})

		for _, topic := range log.Topics {
			logs[index].Topics = append(logs[index].Topics, common.Hash(topic))
		}
	}

	computeEffort, storageConsumed, storageReleased := result.MeterState()
	storageEffort := uint64(0)

	if storageReleased <= storageConsumed {
		storageEffort = storageConsumed - storageReleased
	}

	cr := &engineio.CallResult{
		Out:           result.Output(),
		Logs:          logs,
		ComputeEffort: computeEffort,
		StorageEffort: storageEffort,
	}

	if result.Error() != nil {
		// TODO: check if we need to parse the entire error or just the message
		cr.Err = result.Error().Bytes()
	}

	return cr
}

type TransitionWrapper struct {
	t engineio.Transition
}

func (t *TransitionWrapper) Storage(id [32]byte) (pstorage.Observable, error) {
	return t.t.GetLogicStorageObject(id)
}

func NewTransitionWrapper(t engineio.Transition) *TransitionWrapper {
	return &TransitionWrapper{t: t}
}

type AssetEngineWrapper struct {
	ae engineio.AssetEngine
}

func (aew *AssetEngineWrapper) Transfer(
	assetID [32]byte, tokenID uint64, operatorID [32]byte, benefactorID [32]byte,
	beneficiaryID [32]byte, amount *big.Int, access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Transfer(
		assetID,
		common.TokenID(tokenID),
		operatorID,
		benefactorID,
		beneficiaryID,
		amount,
		access,
	)
}

func (aew *AssetEngineWrapper) Mint(
	assetID [32]byte, tokenID uint64, senderID, beneficiaryID [32]byte, amount *big.Int,
	staticMetadata map[string][]byte, access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Mint(assetID, common.TokenID(tokenID), senderID, beneficiaryID, amount, staticMetadata, access)
}

func (aew *AssetEngineWrapper) Burn(
	assetID [32]byte, tokenID uint64, benefactorID [32]byte, amount *big.Int, access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Burn(
		assetID,
		common.TokenID(tokenID),
		benefactorID,
		amount,
		access,
	)
}

func (aew *AssetEngineWrapper) Approve(
	assetID [32]byte, tokenID uint64,
	benefactorID, beneficiaryID [32]byte, amount *big.Int, expiresAt uint64,
	access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Approve(
		assetID,
		common.TokenID(tokenID),
		benefactorID,
		beneficiaryID,
		amount,
		expiresAt,
		access,
	)
}

func (aew *AssetEngineWrapper) Revoke(
	assetID [32]byte, tokenID uint64,
	benefactorID, beneficiaryID [32]byte,
	access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Revoke(
		assetID,
		common.TokenID(tokenID),
		benefactorID,
		beneficiaryID,
		access,
	)
}

func (aew *AssetEngineWrapper) Lockup(
	assetID [32]byte, tokenID uint64,
	benefactorID, beneficiaryID [32]byte, amount *big.Int, access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Lockup(
		assetID,
		common.TokenID(tokenID),
		benefactorID,
		beneficiaryID,
		amount,
		access,
	)
}

func (aew *AssetEngineWrapper) Release(
	assetID [32]byte, tokenID uint64,
	operatorID, benefactorID, beneficiaryID [32]byte, amount *big.Int, access map[[32]byte]int,
) (uint64, error) {
	return aew.ae.Release(
		assetID,
		common.TokenID(tokenID),
		operatorID,
		benefactorID,
		beneficiaryID,
		amount,
		access,
	)
}

func (aew *AssetEngineWrapper) Symbol(assetID [32]byte, access map[[32]byte]int) (string, uint64, error) {
	symbol, err := aew.ae.Symbol(assetID, access)
	if err != nil {
		return "", 5, err
	}

	return symbol, 5, nil
}

func (aew *AssetEngineWrapper) BalanceOf(
	assetID [32]byte, tokenID uint64, address [32]byte, access map[[32]byte]int,
) (*big.Int, uint64, error) {
	balance, err := aew.ae.BalanceOf(
		address,
		assetID,
		common.TokenID(tokenID),
		access,
	)
	if err != nil {
		return nil, 5, err
	}

	return balance, 5, nil
}

func (aew *AssetEngineWrapper) Creator(assetID [32]byte, access map[[32]byte]int) ([32]byte, uint64, error) {
	creator, err := aew.ae.Creator(assetID, access)
	if err != nil {
		return [32]byte{}, 5, err
	}

	return creator, 5, nil
}

func (aew *AssetEngineWrapper) Manager(assetID [32]byte, access map[[32]byte]int) ([32]byte, uint64, error) {
	manager, err := aew.ae.Manager(assetID, access)
	if err != nil {
		return [32]byte{}, 5, err
	}

	return manager, 5, nil
}

func (aew *AssetEngineWrapper) Decimals(assetID [32]byte, access map[[32]byte]int) (uint8, uint64, error) {
	decimals, err := aew.ae.Decimals(assetID, access)
	if err != nil {
		return 0, 5, err
	}

	return decimals, 5, nil
}

func (aew *AssetEngineWrapper) MaxSupply(assetID [32]byte, access map[[32]byte]int) (*big.Int, uint64, error) {
	maxSupply, err := aew.ae.MaxSupply(assetID, access)
	if err != nil {
		return nil, 5, err
	}

	return maxSupply, 5, nil
}

func (aew *AssetEngineWrapper) CirculatingSupply(assetID [32]byte, access map[[32]byte]int) (*big.Int, uint64, error) {
	circSupply, err := aew.ae.CirculatingSupply(assetID, access)
	if err != nil {
		return nil, 5, err
	}

	return circSupply, 5, nil
}

func (aew *AssetEngineWrapper) LogicID(assetID [32]byte, access map[[32]byte]int) ([32]byte, uint64, error) {
	logicID, err := aew.ae.LogicID(assetID, access)
	if err != nil {
		return [32]byte{}, 5, err
	}

	return logicID, 5, nil
}

func (aew *AssetEngineWrapper) EnableEvents(assetID [32]byte, access map[[32]byte]int) (bool, uint64, error) {
	enableEvents, err := aew.ae.EnableEvents(assetID, access)
	if err != nil {
		return false, 5, err
	}

	return enableEvents, 10, nil
}

func (aew *AssetEngineWrapper) SetStaticMetaData(assetID, participantID [32]byte,
	key string, val []byte, access map[[32]byte]int,
) (uint64, error) {
	err := aew.ae.SetStaticMetaData(assetID, participantID, key, val, access)
	if err != nil {
		return 5, err
	}

	return 10, nil
}

func (aew *AssetEngineWrapper) SetDynamicMetaData(assetID, participantID [32]byte,
	key string, val []byte, access map[[32]byte]int,
) (uint64, error) {
	err := aew.ae.SetDynamicMetaData(assetID, participantID, key, val, access)
	if err != nil {
		return 5, err
	}

	return 10, nil
}

func (aew *AssetEngineWrapper) GetStaticMetaData(assetID [32]byte, key string,
	access map[[32]byte]int,
) ([]byte, uint64, error) {
	metadata, err := aew.ae.GetStaticMetaData(assetID, key, access)
	if err != nil {
		return nil, 10, err
	}

	return metadata, 10, nil
}

func (aew *AssetEngineWrapper) GetDynamicMetaData(
	assetID [32]byte, key string,
	access map[[32]byte]int,
) ([]byte, uint64, error) {
	metadata, err := aew.ae.GetDynamicMetaData(assetID, key, access)
	if err != nil {
		return nil, 10, err
	}

	return metadata, 10, nil
}

func (aew *AssetEngineWrapper) SetStaticTokenMetaData(
	assetID [32]byte, participantID [32]byte,
	tokenID uint64, key string, val []byte, access map[[32]byte]int,
) (uint64, error) {
	err := aew.ae.SetStaticTokenMetaData(assetID, participantID, common.TokenID(tokenID), key, val, access)
	if err != nil {
		return 5, err
	}

	return 10, nil
}

func (aew *AssetEngineWrapper) SetDynamicTokenMetaData(
	assetID [32]byte, participantID [32]byte,
	tokenID uint64, key string, val []byte, access map[[32]byte]int,
) (uint64, error) {
	err := aew.ae.SetDynamicTokenMetaData(assetID, participantID, common.TokenID(tokenID), key, val, access)
	if err != nil {
		return 5, err
	}

	return 10, nil
}

func (aew *AssetEngineWrapper) GetStaticTokenMetaData(
	assetID [32]byte, participantID [32]byte,
	tokenID uint64, key string, access map[[32]byte]int,
) ([]byte, uint64, error) {
	metadata, err := aew.ae.GetStaticTokenMetaData(assetID, participantID, common.TokenID(tokenID), key, access)
	if err != nil {
		return nil, 10, err
	}

	return metadata, 10, nil
}

func (aew *AssetEngineWrapper) GetDynamicTokenMetaData(
	assetID [32]byte, participantID [32]byte,
	tokenID uint64, key string, access map[[32]byte]int,
) ([]byte, uint64, error) {
	metadata, err := aew.ae.GetDynamicTokenMetaData(assetID, participantID, common.TokenID(tokenID), key, access)
	if err != nil {
		return nil, 10, err
	}

	return metadata, 10, nil
}
