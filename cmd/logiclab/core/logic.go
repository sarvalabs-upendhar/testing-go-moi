package core

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/state"
)

// Logic is a container for
// all contents related to a Logic entity
type Logic struct {
	Name  string
	Ready bool

	Object    *state.LogicObject
	Callsites map[string]LogicCallsite
}

type LogicCallsite struct {
	Ptr  uint64 `json:"ptr"`
	Kind string `json:"kind"`
	Sign string `json:"sign"`
}

type LogicMetadata struct {
	LogicID  identifiers.LogicID `json:"logicID"`
	Manifest common.Hash         `json:"manifest"`
}

// FormatREPL returns a string representation of the Logic for compatibility with the REPL
func (logic Logic) FormatREPL() string {
	var str strings.Builder

	logicID := logic.Object.ID
	identifier, _ := logicID.Identifier()

	str.WriteString(fmt.Sprintf("==== [ @>%v<@ ] [Ready: @>%v<@] [Address: %v] \n", logic.Name, logic.Ready, logicID.Address())) //nolint:lll
	str.WriteString(fmt.Sprintf("[Edition: %v] [Logic ID: @>%v<@]\n", identifier.Edition(), logicID))
	str.WriteString(fmt.Sprintf("[Engine: @>%v<@] [Manifest: %x]\n", logic.Object.Engine(), logic.Object.ManifestHash()))
	str.WriteString(fmt.Sprintf(
		"[Persistent: @>%v<@] [Ephemeral: @>%v<@] [Interactive: @>%v<@] [Asset Logic: @>%v<@]\n",
		identifier.HasPersistentState(), identifier.HasEphemeralState(),
		identifier.HasInteractableSites(), identifier.AssetLogic(),
	))

	str.WriteString("\n==== Callsites\n")

	for callsite, object := range logic.Callsites {
		str.WriteString(fmt.Sprintf("[%v][%v]: @>%v<@%v\n", object.Ptr, object.Kind, callsite, object.Sign))
	}

	str.WriteString("====")

	return str.String()
}

func (logic *Logic) Encode() ([]byte, error) {
	rawData, err := polo.Polorize(logic)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize logic account")
	}

	return rawData, nil
}

func (logic *Logic) Decode(bytes []byte) error {
	if err := polo.Depolorize(logic, bytes); err != nil {
		return errors.Wrap(err, "failed to depolorize logic account")
	}

	return nil
}

// LogicExists returns whether a logic with the given name exists in the Environment
func (env *Environment) LogicExists(name string) bool {
	_, exists := env.Logics[name]
	return exists //nolint:nlreturn
}

func (env *Environment) FetchLogic(name string) (*Logic, error) {
	// Check if logic has a cache entry
	if logic, ok := env.lcache[name]; ok {
		return logic, nil
	}

	// Check if a logic with the given name exists
	logicMetaData, ok := env.Logics[name]
	if !ok {
		return nil, ErrLogicNotFound
	}

	// Retrieve the logic from the database
	raw, err := env.database.Get(db.LogicAccountKey(env.ID, logicMetaData.LogicID))
	if err != nil {
		// This should never happen, It means something is
		// seriously wrong with the environment handling
		if errors.Is(err, db.ErrKeyNotFound) {
			panic("known logic not found at expected key")
		}

		return nil, err
	}

	// Decode the value into an Account
	account := new(Account)
	if err = account.Decode(raw); err != nil {
		return nil, err
	}

	// Decode the account data into Logic
	logic := new(Logic)
	if err = logic.Decode(account.Data); err != nil {
		return nil, err
	}

	// Cache the logic data. This is a read-only cache.
	// Logic contains no mutable information so, it will never be updated in the database
	env.lcache[name] = logic

	return logic, nil
}

// CompileLogic adds a Logic object to the Environment.
// The logic is indexed by its name.
func (env *Environment) CompileLogic(
	name string,
	manifest engineio.Manifest,
	fuel engineio.EngineFuel,
) (*Logic, engineio.EngineFuel, error) {
	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngine(manifest.Engine().Kind)
	if !ok {
		return nil, 0, errors.Errorf("unsupported manifest engine: %v", manifest.Engine().Kind)
	}

	// Compile the manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(manifest, fuel)
	if err != nil {
		return nil, consumed, err
	}

	logicAddress := identifiers.NewRandomAddress()

	for env.AddrExists(logicAddress) {
		logicAddress = identifiers.NewRandomAddress()
	}
	// Create a new LogicObject from the LogicDescriptor
	// A random address is assigned to the logic account
	logicObject := state.NewLogicObject(logicAddress, descriptor)

	// If the logic ID has no persistent state, it can be marked
	// as ready, otherwise it requires a deployment to occur first
	id, _ := logicObject.ID.Identifier()
	ready := !id.HasPersistentState()

	logic := &Logic{
		Name:      name,
		Ready:     ready,
		Object:    logicObject,
		Callsites: make(map[string]LogicCallsite),
	}

	for site, object := range logicObject.Callsites {
		logic.Callsites[site] = LogicCallsite{
			Kind: object.Kind.String(),
			Ptr:  object.Ptr,
			Sign: generateSignatures(manifest, object),
		}
	}

	env.Addrs[logicObject.ID.Address()] = struct{}{}
	env.lcache[logic.Name] = logic
	env.Logics[logic.Name] = LogicMetadata{
		LogicID:  logicObject.ID,
		Manifest: manifest.Hash(),
	}

	encoded, err := logic.Encode()
	if err != nil {
		return nil, 0, err
	}

	account := &Account{
		Kind: LogicAccount,
		Name: logic.Name,
		Data: encoded,
	}

	rawAccount, err := account.Encode()
	if err != nil {
		return nil, 0, err
	}

	if err = env.database.Set(db.LogicAccountKey(env.ID, logicObject.ID), rawAccount); err != nil {
		return nil, 0, err
	}

	encoded, err = manifest.Encode(common.POLO)
	if err != nil {
		return nil, 0, err
	}

	if err = env.database.Set(db.LogicManifestKey(env.ID, logicObject.ID), encoded); err != nil {
		return nil, 0, err
	}

	return logic, consumed, nil
}

// RemoveLogic removes a logic from the Environment with a given name.
func (env *Environment) RemoveLogic(name string) error {
	logicMetaData, ok := env.Logics[name]
	if !ok {
		return ErrLogicNotFound
	}

	delete(env.lcache, name)
	delete(env.Logics, name)

	// Delete all keys in logic's address subspace
	// This includes the logic entity, the logic manifest
	// as well as any persistent state storage of the logic
	if err := env.database.PrefixDelete(db.AccountPrefix(env.ID, logicMetaData.LogicID.Address())); err != nil {
		return err
	}

	// todo: ephemeral state storage cleanup
	// we should remove storage from all users for this logic

	return nil
}

// PurgeLogics removes all users from the Environment
func (env *Environment) PurgeLogics() error {
	// Iterate over the logics and remove each one
	for name := range env.Logics {
		if err := env.RemoveLogic(name); err != nil {
			return err
		}
	}

	// Reset the logic registry and cache
	env.Logics = make(map[string]LogicMetadata)
	env.lcache = make(map[string]*Logic)

	return nil
}

func generateSignatures(manifest engineio.Manifest, callsite engineio.Callsite) string {
	if manifest.Engine().Kind != engineio.PISA {
		return "() -> ()"
	}

	element := manifest.Elements()[callsite.Ptr]
	routine, _ := element.Data.(*pisa.RoutineSchema)

	inputs := make([]string, 0, len(routine.Accepts))
	for _, field := range routine.Accepts {
		inputs = append(inputs, fmt.Sprintf("%v %v", field.Label, field.Type))
	}

	outputs := make([]string, 0, len(routine.Returns))

	for _, field := range routine.Returns {
		outputs = append(outputs, fmt.Sprintf("%v %v", field.Label, field.Type))
	}

	signature := fmt.Sprintf("(%v) -> (%v)", strings.Join(inputs, ", "), strings.Join(outputs, ", "))

	return signature
}
