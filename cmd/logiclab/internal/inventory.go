package internal

import (
	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/compute/engineio"
)

// Inventory represents an inventory manager
// for all saved items handled by the LogicLab
type Inventory struct {
	labdir string
	pcache map[string]*ParticipantState
	lcache map[string]*LogicAccountState

	Config LabConfig

	Sender   string
	Receiver string

	Participants  map[string]common.Address
	LogicAccounts map[string]common.LogicID
}

type LabConfig struct {
	BaseFuel engineio.Fuel

	HexBig   bool
	HexBytes bool
}

// ParticipantExists returns whether a participant with the given name exists in the Inventory
func (inventory *Inventory) ParticipantExists(username string) bool {
	_, exists := inventory.Participants[username]

	return exists
}

// LogicExists returns whether a logic with the given name exists in the Inventory
func (inventory *Inventory) LogicExists(name string) bool {
	_, exists := inventory.LogicAccounts[name]

	return exists
}

// AddParticipant adds a ParticipantState object to the Inventory.
// The participant is indexed by their username.
func (inventory *Inventory) AddParticipant(participant *ParticipantState) {
	inventory.pcache[participant.Username] = participant
	inventory.Participants[participant.Username] = participant.Address
}

// AddLogic adds a LogicAccountState object to the Inventory.
// The logic is indexed by its name.
func (inventory *Inventory) AddLogic(logic *LogicAccountState) {
	inventory.lcache[logic.Name] = logic
	inventory.LogicAccounts[logic.Name] = logic.Object.LogicID()
}

// RemoveParticipant removes a participant from the Inventory with a given username.
func (inventory *Inventory) RemoveParticipant(username string) {
	delete(inventory.pcache, username)
	delete(inventory.Participants, username)
	_ = deleteFile(participantFilename(inventory.labdir, username))
}

// RemoveLogic removes a logic from the Inventory with a given name.
func (inventory *Inventory) RemoveLogic(name string) {
	delete(inventory.lcache, name)
	delete(inventory.LogicAccounts, name)
	_ = deleteFile(logicFilename(inventory.labdir, name))
}

// FindParticipant attempts to find a participant and return their ParticipantState for a given username.
// Fails if no such participant is tracked by the Inventory or if the file for a known participant is missing.
func (inventory *Inventory) FindParticipant(username string) (*ParticipantState, bool) {
	// Check if participant exists
	if exists := inventory.ParticipantExists(username); !exists {
		return nil, false
	}

	// Check if known participant is available in the cache
	if participant, cached := inventory.pcache[username]; cached {
		return participant, true
	}

	// Generate filename for participant
	filename := participantFilename(inventory.labdir, username)
	// Load participant from file
	participant := new(ParticipantState)
	if err := loadFile(filename, participant); err != nil {
		// If the file for a known participant
		// is missing, forget the participant
		if errors.Is(err, ErrMissingFile) {
			delete(inventory.Participants, username)
		}

		return nil, false
	}

	// Store the retrieved ParticipantState to the inventory cache
	inventory.pcache[username] = participant

	return participant, true
}

// FindLogic attempts to find a logic and return their LogicAccountState for a given name.
// Fails if no such logic is tracked by the Inventory or if the file for a known logic is missing.
func (inventory *Inventory) FindLogic(name string) (*LogicAccountState, bool) {
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
	logic := new(LogicAccountState)
	if err := loadFile(filename, logic); err != nil {
		// If the file for a known logic
		// is missing, forget the logic
		if errors.Is(err, ErrMissingFile) {
			delete(inventory.LogicAccounts, name)
		}

		return nil, false
	}

	// Store the retrieved LogicAccountState to the inventory cache
	inventory.lcache[name] = logic

	return logic, true
}

// load retrieves the contents of the inventory.json file at
// the given directory path and loads it into the calling object
func (inventory *Inventory) load(dirpath string) error {
	// Create new Inventory object and decode the file into it
	decoded := new(Inventory)
	if err := loadFile(inventoryFilename(dirpath), decoded); err != nil {
		if errors.Is(err, ErrMissingFile) {
			return errors.New("inventory file missing")
		}

		return errors.Wrap(err, "failed to read inventory file")
	}

	*inventory = *decoded

	inventory.labdir = dirpath
	inventory.pcache = make(map[string]*ParticipantState)
	inventory.lcache = make(map[string]*LogicAccountState)

	return nil
}

// save writes the Inventory object to a specified directory
func (inventory *Inventory) save() error {
	// Save every participant in the cache
	for _, participant := range inventory.pcache {
		if err := saveFile(participantFilename(inventory.labdir, participant.Username), participant); err != nil {
			return errors.Wrapf(err, "failed to save participant '%v'", participant.Username)
		}
	}

	// Save every logic in the cache
	for _, logic := range inventory.lcache {
		if err := saveFile(logicFilename(inventory.labdir, logic.Name), logic); err != nil {
			return errors.Wrapf(err, "failed to save logic '%v'", logic.Name)
		}
	}

	if err := saveFile(inventoryFilename(inventory.labdir), inventory); err != nil {
		return errors.Wrap(err, "failed to write inventory file")
	}

	return nil
}
