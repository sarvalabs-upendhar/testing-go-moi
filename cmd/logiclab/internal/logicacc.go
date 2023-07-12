package internal

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/compute/engineio"
	gtypes "github.com/sarvalabs/go-moi/state"
)

// LogicAccountState is a container for
// all contents related to a Logic entity
type LogicAccountState struct {
	Name     string
	Ready    bool
	Manifest *engineio.Manifest

	Object   *gtypes.LogicObject
	CtxState *StateObject
}

// String returns a string representation of the LogicAccountState.
// Implements the Stringer interface for LogicAccountState.
func (logic LogicAccountState) String() string {
	var str strings.Builder

	logicID := logic.Object.LogicID()
	identifier, _ := logicID.Identifier()

	str.WriteString(fmt.Sprintf("==== [ %v ] [Address: %v] [Ready: %v]\n", logic.Name, logicID.Address(), logic.Ready))
	str.WriteString(fmt.Sprintf("[Edition: %v] [Logic ID: %v]\n", identifier.Edition(), logicID))
	str.WriteString(fmt.Sprintf("[Engine: %v] [Manifest: %v]\n", logic.Object.Engine(), logic.Object.Manifest()))
	str.WriteString(fmt.Sprintf(
		"[Persistent: %v] [Ephemeral: %v] [Interactive: %v] [Asset Logic: %v]\n",
		identifier.PersistentState(), identifier.EphemeralState(), identifier.Interactive(), identifier.AssetLogic()),
	)

	str.WriteString("\n==== Callsites\n")

	for callsite, object := range logic.Object.Callsites {
		str.WriteString(fmt.Sprintf("[%v] %v [%v]\n", object.Ptr, callsite, object.Kind))
	}

	str.WriteString("====\n")
	str.WriteString("\n==== State\n")

	for key, val := range logic.CtxState.LogicState[logic.Object.ID.String()] {
		str.WriteString(fmt.Sprintf("%v: %v\n", key, hex.EncodeToString(val)))
	}

	str.WriteString("====")

	return str.String()
}

// LogicCompileFromManifestCommand generates a Command runner to
// compile a new Logic with the given name and manifest file path
func LogicCompileFromManifestCommand(name string, manifest *engineio.Manifest) Command {
	return func(env *Environment) string {
		// Check if a logic with name already exists
		if _, exists := env.inventory.LogicAccounts[name]; exists {
			return fmt.Sprintf("logic %v already exists", name)
		}

		// Compile the manifest into a Logic
		fuel, logic, err := CompileManifestFile(manifest, name, env.inventory.Config.BaseFuel)
		if err != nil {
			return fmt.Sprintf("logic could not be compiled: %v", err)
		}

		// Add the logic to the inventory
		env.inventory.AddLogic(logic)

		return fmt.Sprintf("logic '%v' [%v] compiled with %v FUEL", name, logic.Object.ID, fuel)
	}
}

// LogicDeleteCommand generates a Command runner
// to delete an existing Logic with the given name
func LogicDeleteCommand(name string) Command {
	return func(env *Environment) string {
		// Check if a logic with name exists
		if exists := env.inventory.LogicExists(name); !exists {
			return fmt.Sprintf("logic %v does not exist", name)
		}

		// Remove the logic from the inventory
		env.inventory.RemoveLogic(name)

		return fmt.Sprintf("logic '%v' removed", name)
	}
}

// LogicInspectCommand generates a Command runner to print
// the details of a specific logic with a given name
func LogicInspectCommand(name string) Command {
	return func(env *Environment) string {
		// Find the logic in the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic %v does not exist", name)
		}

		return logic.String()
	}
}

// LogicListCommand generates a Command runner
// to print details of all compiled logics
func LogicListCommand() Command {
	return func(env *Environment) string {
		var (
			idx  = 1
			list strings.Builder
		)

		for name, logicID := range env.inventory.LogicAccounts {
			list.WriteString(fmt.Sprintf("%v] %v [%v]", idx, name, logicID))

			if idx++; idx <= len(env.inventory.LogicAccounts) {
				list.WriteString("\n")
			}
		}

		if idx == 1 {
			list.WriteString("no logics found")
		}

		return list.String()
	}
}

// CompileManifestFile reads and compiles a manifest file at the given path into a Logic with the given name.
func CompileManifestFile(manifest *engineio.Manifest, name string, fuel engineio.Fuel) (
	engineio.Fuel, *LogicAccountState, error,
) {
	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngineRuntime(manifest.Header().LogicEngine())
	if !ok {
		return nil, nil, errors.Errorf("unsupported manifest engine: %v", manifest.Header().LogicEngine())
	}

	// Compile the manifest into a LogicDescriptor
	descriptor, consumed, err := runtime.CompileManifest(fuel, manifest)
	if err != nil {
		return consumed, nil, err
	}

	// Create a new Object
	logicCtxState := NewStateObject(randomAddress())
	// Create a new LogicObject from the LogicDescriptor
	logicObject := gtypes.NewLogicObject(logicCtxState.Address, descriptor)

	return consumed, &LogicAccountState{
		Name:     name,
		Manifest: manifest,
		Object:   logicObject,
		CtxState: logicCtxState,

		// If the logic ID has no persistent state, it can be marked
		// as ready, otherwise it requires a deploy to occur first
		Ready: !must(logicObject.LogicID().Identifier()).PersistentState(),
	}, nil
}

func must[T any](object T, err error) T {
	if err != nil {
		panic(err)
	}

	return object
}
