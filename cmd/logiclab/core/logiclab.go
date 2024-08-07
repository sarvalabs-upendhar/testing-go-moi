package core

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

var (
	ErrUnsupportedEngine = errors.New("unsupported engine")

	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrLogicAlreadyExists = errors.New("logic already exists")
	ErrAddrAlreadyExists  = errors.New("address already exists")
	ErrEnvAlreadyExists   = errors.New("environment already exists")

	ErrUserNotFound         = errors.New("user not found")
	ErrLogicNotFound        = errors.New("logic not found")
	ErrEnvironmentNotFound  = errors.New("environment not found")
	ErrAddrNotFound         = errors.New("address not found")
	ErrStorageValueNotFound = errors.New("storage value not found")

	ErrSenderNotConf     = errors.New("sender not configured for environment")
	ErrSenderAlreadyConf = errors.New("sender already configured for environment")
)

var Engines = []engineio.EngineKind{
	engineio.PISA,
}

const (
	DefaultDir  = "./.logiclab"
	DefaultEnv  = "main"
	DefaultPort = 6060
)

type Lab struct {
	Database db.Database
	envcache map[string]*Environment
}

// NewLab creates a new LogicLab instance at the specified directory.
// If no logiclab database exists at the directory, it will be initialized
func NewLab(dirpath string) (*Lab, error) {
	database, err := db.NewBadgerDatabase(dirpath)
	if err != nil {
		return nil, err
	}

	lab := &Lab{
		Database: database,
		envcache: make(map[string]*Environment),
	}

	// Add the default environment to the lab
	// If the environment already exists, this is a no-op
	if err = lab.AddEnvironment(DefaultEnv); err != nil {
		if !errors.Is(err, ErrEnvAlreadyExists) {
			return nil, err
		}
	}

	return lab, nil
}

func (lab *Lab) AddEnvironment(name string) error {
	// Check if an environment with the name already exists
	ok, err := lab.HasEnvironment(name)
	if err != nil {
		return err
	}

	if ok {
		return ErrEnvAlreadyExists
	}

	// Create a new environment
	env := NewEnvironment(name, lab.Database)
	lab.envcache[name] = env

	return nil
}

func (lab *Lab) HasEnvironment(env string) (bool, error) {
	if _, ok := lab.envcache[env]; ok {
		return true, nil
	}

	// Check if the environment exists
	exists, err := lab.Database.Has(db.EnvironmentKey(env))
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (lab *Lab) GetEnvironment(env string) (*Environment, bool, error) {
	if environment, ok := lab.envcache[env]; ok {
		return environment, true, nil
	}

	// Get the raw environment from the db
	raw, err := lab.Database.Get(db.EnvironmentKey(env))
	if err != nil {
		if errors.Is(err, db.ErrKeyNotFound) {
			return nil, false, nil
		}

		return nil, false, err
	}

	// Decode the value into an Environment
	environment := new(Environment)
	// Attach the db client to the environment
	environment.database = lab.Database

	if err = environment.Decode(raw); err != nil {
		return nil, true, err
	}

	// Cache the environment
	lab.envcache[env] = environment

	return environment, true, nil
}

func (lab *Lab) ListEnvironments() ([]string, error) {
	// Get all entries in the db with 'environ' prefix
	entries, err := lab.Database.PrefixCollect(db.TagEnviron)
	if err != nil {
		return nil, err
	}

	environments := make(map[string]struct{})
	// Collect all the environments (trim the prefix)
	for key := range entries {
		environments[strings.TrimPrefix(key, "environ-")] = struct{}{}
	}

	// Collect all the environments from the envcache
	for name := range lab.envcache {
		environments[name] = struct{}{}
	}

	envs := make([]string, 0)
	// Flatten the map into a slice
	for name := range environments {
		envs = append(envs, name)
	}

	return envs, nil
}

func (lab *Lab) DelEnvironment(env string) error {
	// Check if the environment exists in envcache
	if _, ok := lab.envcache[env]; ok {
		// Environment found in envcache, delete it
		delete(lab.envcache, env)

		return nil
	}

	// Check if the environment exists in the database
	exists, err := lab.Database.Has(db.EnvironmentKey(env))
	if err != nil {
		return err
	}

	if !exists {
		return ErrEnvironmentNotFound
	}

	// Delete the environment key from the database
	if err = lab.Database.Del(db.EnvironmentKey(env)); err != nil {
		return err
	}

	// Delete all keys from the database with prefix of the environment name
	if err = lab.Database.PrefixDelete(db.EnvironmentPrefix(env)); err != nil {
		return err
	}

	return nil
}

// ClusterID implements the engineio.EnvDriver for Environment.
// Returns the "LogicLab" constant.
func (lab *Lab) ClusterID() string { return "LogicLab" }

// Timestamp implements the engineio.EnvDriver for Environment.
// Returns the current unix timestamp.
func (lab *Lab) Timestamp() uint64 {
	return uint64(time.Now().Unix())
}

func (lab *Lab) HandleInterrupt() func() {
	// Setup a channel to receive interrupt signals
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Return a function to be executed by defer
	return func() {
		<-interrupt

		_ = lab.Close()

		os.Exit(0)
	}
}

// Close exits the logiclab environment
func (lab *Lab) Close() error {
	for id, env := range lab.envcache {
		encoded, err := env.Encode()
		if err != nil {
			return errors.Wrap(err, "unable to encode environment")
		}

		if err = lab.Database.Set(db.EnvironmentKey(id), encoded); err != nil {
			return errors.Wrap(err, "unable to save environments to db")
		}
	}

	if err := lab.Database.Close(); err != nil {
		return err
	}

	return nil
}
