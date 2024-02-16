package core

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
)

const (
	LabFuelPrice uint64 = 1
	LabBaseFuel  uint64 = 10000
)

// Inventory represents an inventory manager
// for all saved items handled by the LogicLab
type Inventory struct {
	labdir string

	ucache map[string]*UserAccount
	lcache map[string]*LogicAccount

	Config LabConfig

	Sender   string
	Receiver string

	Users  map[string]identifiers.Address
	Logics map[string]identifiers.LogicID
}

type LabConfig struct {
	HexBigInt bool
	HexBytes  bool
	BaseFuel  uint64
}

func FreshInventory(dirpath string) Inventory {
	return Inventory{
		labdir: dirpath,
		Config: LabConfig{
			BaseFuel:  LabBaseFuel,
			HexBigInt: true,
			HexBytes:  true,
		},

		Users:  make(map[string]identifiers.Address),
		Logics: make(map[string]identifiers.LogicID),
	}
}

// Dir returns the lab directory that the inventory is stored at
func (inventory *Inventory) Dir() string { return inventory.labdir }

// Load retrieves the contents of the inventory.json file at
// the given directory path and loads it into the calling object
func (inventory *Inventory) Load(dirpath string) error {
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
	inventory.ucache = make(map[string]*UserAccount)
	inventory.lcache = make(map[string]*LogicAccount)

	return nil
}

// Save writes the Inventory object to a specified directory
func (inventory *Inventory) Save() error {
	// Save every user in the cache
	for _, user := range inventory.ucache {
		if err := saveFile(userFilename(inventory.labdir, user.Name), user); err != nil {
			return errors.Wrapf(err, "failed to save participant '%v'", user.Name)
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
