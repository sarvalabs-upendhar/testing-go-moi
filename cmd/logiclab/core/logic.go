package core

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-pisa"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
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

// NewLogic reads and compiles a manifest file at the given path into a Logic with the given name.
func NewLogic(name string, manifest *engineio.Manifest, fuel engineio.EngineFuel) (*Logic, engineio.EngineFuel, error) {
	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	if !ok {
		return nil, 0, errors.Errorf("unsupported manifest engine: %v", manifest.Header().LogicEngine())
	}

	// Compile the manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(fuel, manifest)
	if err != nil {
		return nil, consumed, err
	}

	// Create a new LogicObject from the LogicDescriptor
	// A random address is assigned to the logic account
	logicObject := state.NewLogicObject(identifiers.NewRandomAddress(), descriptor)

	// If the logic ID has no persistent state, it can be marked
	// as ready, otherwise it requires a deploy to occur first
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

	return logic, consumed, nil
}

// FormatREPL returns a string representation of the Logic for compatibility with the REPL
func (logic Logic) FormatREPL() string {
	var str strings.Builder

	logicID := logic.Object.ID
	identifier, _ := logicID.Identifier()

	str.WriteString(fmt.Sprintf("==== [ @>%v<@ ] [Ready: @>%v<@] [Address: %v] \n", logic.Name, logic.Ready, logicID.Address())) //nolint:lll
	str.WriteString(fmt.Sprintf("[Edition: %v] [Logic ID: @>%v<@]\n", identifier.Edition(), logicID))
	str.WriteString(fmt.Sprintf("[Engine: @>%v<@] [Manifest: %x]\n", logic.Object.Engine(), logic.Object.Manifest()))
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
	logicID, ok := env.Logics[name]
	if !ok {
		return nil, ErrLogicNotFound
	}

	// Retrieve the logic from the database
	raw, err := env.database.Get(db.LogicEntityKey(env.ID, logicID))
	if err != nil {
		// This should never happen, It means something is
		// seriously wrong with the environment handling
		if errors.Is(err, db.ErrKeyNotFound) {
			panic("known logic not found at expected key")
		}

		return nil, err
	}

	// Decode the value into a Logic
	logic := new(Logic)
	if err = logic.Decode(raw); err != nil {
		return nil, err
	}

	// Cache the logic data. This is a read-only cache.
	// Logic contains no mutable information so, it will never be updated in the database
	env.lcache[name] = logic

	return logic, nil
}

// RegisterLogic adds a Logic object to the Environment.
// The logic is indexed by its name.
func (env *Environment) RegisterLogic(logic *Logic, manifest *engineio.Manifest) error {
	// Check if user with the given name already exists
	if env.LogicExists(logic.Name) {
		return ErrLogicAlreadyExists
	}

	logicID := logic.Object.ID

	env.lcache[logic.Name] = logic
	env.Logics[logic.Name] = logicID

	encoded, err := logic.Encode()
	if err != nil {
		return err
	}

	if err = env.database.Set(db.LogicEntityKey(env.ID, logicID), encoded); err != nil {
		return err
	}

	encoded, err = manifest.Encode(engineio.POLO)
	if err != nil {
		return err
	}

	if err = env.database.Set(db.LogicManifestKey(env.ID, logicID), encoded); err != nil {
		return err
	}

	return nil
}

// RemoveLogic removes a logic from the Environment with a given name.
func (env *Environment) RemoveLogic(name string) error {
	logicID, ok := env.Logics[name]
	if !ok {
		return ErrLogicNotFound
	}

	delete(env.lcache, name)
	delete(env.Logics, name)

	// Delete all keys in logic's address subspace
	// This includes the logic entity, the logic manifest
	// as well as any persistent state storage of the logic
	if err := env.database.PrefixDelete(db.AddressPrefix(env.ID, logicID.Address())); err != nil {
		return err
	}

	// todo: ephemeral state storage cleanup
	// we should remove storage from all users for this logic

	return nil
}

// RemoveAllLogics removes all users from the Environment
func (env *Environment) RemoveAllLogics() error {
	// Iterate over the logics and remove each one
	for name := range env.Logics {
		if err := env.RemoveLogic(name); err != nil {
			return err
		}
	}

	// Reset the logic registry and cache
	env.Logics = make(map[string]identifiers.LogicID)
	env.lcache = make(map[string]*Logic)

	return nil
}

func generateSignatures(manifest *engineio.Manifest, callsite *engineio.Callsite) string {
	if manifest.Header().LogicEngine() != engineio.PISA {
		return "() -> ()"
	}

	element := manifest.Elements[callsite.Ptr]
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

func EndpointFromString(endpoint string) engineio.CallsiteKind {
	switch endpoint {
	case "DEPLOY":
		return engineio.DeployerCallsite
	case "INVOKE":
		return engineio.InvokableCallsite
	case "ENLISTER":
		return engineio.EnlisterCallsite
	case "INTERACTABLE":
		return engineio.InteractableCallsite
	case "LOCAL":
		return engineio.LocalCallsite
	default:
		panic("unhandled logic call case")
	}
}
