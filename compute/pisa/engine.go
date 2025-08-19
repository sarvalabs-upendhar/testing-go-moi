package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	pisa "github.com/sarvalabs/go-pisa"
	"github.com/sarvalabs/go-pisa/metering"
)

type RuntimeWrapper struct {
	rn *pisa.Runtime
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
	return NewRuntimeWrapper(pisa.NewRuntime(timestamp, 0, 0, pisa.WithCryptography(Crypto(0))))
}

func (p *Pisa) CompileManifest(
	manifest engineio.Manifest,
	fuel engineio.FuelGauge,
) (
	[]byte,
	*engineio.FuelGauge,
	error,
) {
	// Check that the Manifest Instance is PISA
	if manifest.Engine().Kind != engineio.PISA {
		return nil, nil, errors.New("invalid manifest: manifest engine is not PISA")
	}

	// Create a new manifest compiler
	compiler := NewManifestCompiler(fuel, manifest)
	// Compile the manifest
	descriptor, err := compiler.CompileArtifact()
	if err != nil {
		return nil, fuel.Consumed(compiler.fuel), errors.Wrap(err, "compile error")
	}

	return descriptor, fuel.Consumed(compiler.fuel), nil
}

func NewRuntimeWrapper(rn *pisa.Runtime) *RuntimeWrapper {
	return &RuntimeWrapper{rn: rn}
}

func (rw *RuntimeWrapper) CreateLogic(
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
	limit *engineio.FuelGauge,
) *engineio.CallResult {
	result := rw.rn.Call(
		logicID,
		action,
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
