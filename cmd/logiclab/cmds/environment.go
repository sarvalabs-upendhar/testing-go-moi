package cmds

import (
	"time"

	"github.com/pkg/errors"
	engineio "github.com/sarvalabs/go-moi-engineio"
	"go.uber.org/atomic"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type Environment struct {
	// abort indicates if the environment has been aborted
	abort atomic.Bool
	// replOn indicates if a REPL is running in the environment
	replOn atomic.Bool

	// memory contains any runtime memory values
	memory map[string]any
	// inventory contains all the entities the lab
	// has access to, such as users and logics
	inventory *core.Inventory
}

func (env *Environment) Abort()        { env.abort.Store(true) }
func (env *Environment) Aborted() bool { return env.abort.Load() }
func (env *Environment) Active() bool  { return env.replOn.Load() }
func (env *Environment) Activate()     { env.replOn.Store(true) }
func (env *Environment) Deactivate()   { env.replOn.Store(false) }

// LoadEnv loads an existing LogicLab environment.
// Fails it there isn't a directory at the given path with an inventory.json file in it.
func LoadEnv(dirpath string) (*Environment, error) {
	if !core.PathExists(dirpath) {
		return nil, errors.Errorf("could not start LogicLab at directory '%v': directory does not exist", dirpath)
	}

	inventory := new(core.Inventory)
	if err := inventory.Load(dirpath); err != nil {
		return nil, errors.Wrap(err, "could not start LogicLab at directory '%v':")
	}

	return &Environment{
		inventory: inventory,
		memory:    make(map[string]any),
	}, nil
}

// InitEnv initializes a new LogicLab environment.
// Fails if there already exists a directory at the given path.
// The directory is created and initialized with a new inventory.json file.
func InitEnv(dirpath string) error {
	if core.PathExists(dirpath) {
		return errors.Errorf("could not initialize LogicLab directory at '%v': directory already exists", dirpath)
	}

	if err := core.CreateDir(dirpath); err != nil {
		return errors.Wrapf(err, "could not initialize LogicLab directory at '%v'", dirpath)
	}

	inventory := core.FreshInventory(dirpath)

	if err := inventory.Save(); err != nil {
		return errors.Wrap(err, "could not initialize LogicLab directory: failed to create inventory file")
	}

	return nil
}

// GetReference implements the engineio.ReferenceProvider for Environment
func (env *Environment) GetReference(ref engineio.ReferenceVal) (any, bool) {
	val, ok := env.memory[string(ref)]

	return val, ok
}

// ClusterID implements the engineio.EnvDriver for Environment.
// Returns the "LogicLab" constant.
// todo: read this from the config
func (env *Environment) ClusterID() string {
	return "LogicLab"
}

// Timestamp implements the engineio.EnvDriver for Environment.
// Returns the current unix timestamp.
func (env *Environment) Timestamp() int64 {
	return time.Now().Unix()
}

// Close exits the environment by saving the lab session to the inventory
func (env *Environment) Close(message string) error {
	if env.Active() {
		println(message)
	}

	// Flush the inventory file to the directory
	if err := env.inventory.Save(); err != nil {
		return err
	}

	return nil
}
