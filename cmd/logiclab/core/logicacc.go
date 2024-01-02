package core

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"
	"github.com/sarvalabs/go-pisa"

	"github.com/sarvalabs/go-moi/state"
)

// LogicAccount is a container for
// all contents related to a Logic entity
type LogicAccount struct {
	Name  string
	Ready bool

	State    *AccountState
	Logic    *state.LogicObject
	Manifest *engineio.Manifest
}

// String returns a string representation of the LogicAccountState.
// Implements the Stringer interface for LogicAccount.
func (logic LogicAccount) String() string {
	var str strings.Builder

	logicID := logic.Logic.ID
	identifier, _ := logicID.Identifier()

	str.WriteString(fmt.Sprintf("==== [ @>%v<@ ] [Ready: @>%v<@] [Address: %v] \n", logic.Name, logic.Ready, logicID.Address())) //nolint:lll
	str.WriteString(fmt.Sprintf("[Edition: %v] [Logic ID: @>%v<@]\n", identifier.Edition(), logicID))
	str.WriteString(fmt.Sprintf("[Engine: @>%v<@] [Manifest: %x]\n", logic.Logic.Engine(), logic.Logic.Manifest()))
	str.WriteString(fmt.Sprintf(
		"[Persistent: @>%v<@] [Ephemeral: @>%v<@] [Interactive: @>%v<@] [Asset Logic: @>%v<@]\n",
		identifier.HasPersistentState(), identifier.HasEphemeralState(),
		identifier.HasInteractableSites(), identifier.AssetLogic(),
	))

	str.WriteString("\n==== Callsites\n")

	for callsite, object := range logic.Logic.Callsites {
		str.WriteString(logic.generateCallsiteSignature(callsite, object))
	}

	str.WriteString("====\n")
	str.WriteString("\n==== State\n")

	for key, val := range logic.State.Storage[logic.Logic.ID.String()] {
		str.WriteString(fmt.Sprintf("%v: %v\n", key, hex.EncodeToString(val)))
	}

	str.WriteString("====")

	return str.String()
}

// AddLogic adds a core.LogicAccount object to the Inventory.
// The logic is indexed by its name.
func (inventory *Inventory) AddLogic(logic *LogicAccount) {
	inventory.lcache[logic.Name] = logic
	inventory.Logics[logic.Name] = logic.Logic.ID
}

// RemoveLogic removes a logic from the Inventory with a given name.
func (inventory *Inventory) RemoveLogic(name string) {
	delete(inventory.lcache, name)
	delete(inventory.Logics, name)
	_ = deleteFile(logicFilename(inventory.labdir, name))
}

// LogicExists returns whether a logic with the given name exists in the Inventory
func (inventory *Inventory) LogicExists(name string) bool {
	_, exists := inventory.Logics[name]

	return exists
}

// FindLogic attempts to find a logic and return their LogicAccountState for a given name.
// Fails if no such logic is tracked by the Inventory or if the file for a known logic is missing.
func (inventory *Inventory) FindLogic(name string) (*LogicAccount, bool) {
	// Check if logic exists
	if exists := inventory.LogicExists(name); !exists {
		return nil, false
	}

	// Check if known logic is available in the cache
	if logic, cached := inventory.lcache[name]; cached {
		return logic, true
	}

	// Generate filename for logic
	filename := logicFilename(inventory.labdir, name)
	// Load logic from file
	logic := new(LogicAccount)
	if err := loadFile(filename, logic); err != nil {
		// If the file for a known logic
		// is missing, forget the logic
		if errors.Is(err, ErrMissingFile) {
			delete(inventory.Logics, name)
		}

		return nil, false
	}

	// Store the retrieved LogicAccountState to the inventory cache
	inventory.lcache[name] = logic

	return logic, true
}

func (logic LogicAccount) generateCallsiteSignature(site string, object *engineio.Callsite) string {
	if logic.Manifest.Header().LogicEngine() != engineio.PISA {
		return fmt.Sprintf("[%v] [%v] @>%v<@ \n", object.Ptr, object.Kind, site)
	}

	element := logic.Manifest.Elements[object.Ptr]
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

	return fmt.Sprintf("[%v] [%v] @>%v<@%v\n", object.Ptr, object.Kind, site, signature)
}
