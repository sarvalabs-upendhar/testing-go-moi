package core

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
)

const (
	LabFuelPrice   uint64 = 1
	LabDefaultFuel uint64 = 10000
)

// Environment represents an environment for LogicLab
// and acts as a namespace for entities to exist within
type Environment struct {
	database db.Database
	lcache   map[string]*Logic

	ID string

	Addrs  map[identifiers.Address]struct{}
	Users  map[string]identifiers.Address
	Logics map[string]identifiers.LogicID

	Sender   string
	Receiver string
	CallFuel uint64

	Config ReplConfig
}

type ReplConfig struct {
	HexBigInt bool
	HexBytes  bool
}

func NewEnvironment(name string, database db.Database) *Environment {
	return &Environment{
		database: database,
		lcache:   make(map[string]*Logic),

		ID:       name,
		Addrs:    map[identifiers.Address]struct{}{},
		Users:    make(map[string]identifiers.Address),
		Logics:   make(map[string]identifiers.LogicID),
		CallFuel: LabDefaultFuel,

		Config: ReplConfig{
			HexBigInt: true,
			HexBytes:  true,
		},
	}
}

func (env *Environment) Encode() ([]byte, error) {
	encoded, err := polo.Polorize(env)
	if err != nil {
		return nil, errors.Wrap(err, "failed to polorize environment")
	}

	return encoded, nil
}

func (env *Environment) Decode(encoded []byte) error {
	if err := polo.Depolorize(env, encoded); err != nil {
		return errors.Wrap(err, "failed to depolorize environment")
	}

	// Initialize the logic cache
	env.lcache = make(map[string]*Logic)

	return nil
}

// AddrExists returns whether an account with the given address exists in the Environment
func (env *Environment) AddrExists(addr identifiers.Address) bool {
	_, exists := env.Addrs[addr]

	return exists
}

// UserExists returns whether a user with the given name exists in the Environment
func (env *Environment) UserExists(name string) bool {
	_, exists := env.Users[name]

	return exists
}

// RegisterUser registers a User object to the Environment.
// The user is indexed by their username.
func (env *Environment) RegisterUser(name string, addr identifiers.Address) error {
	// Check if user with the given name already exists
	if env.UserExists(name) {
		return ErrUserAlreadyExists
	}

	if addr == identifiers.NilAddress {
		addr = identifiers.NewRandomAddress()
	} else if env.AddrExists(addr) {
		return ErrAddrAlreadyExists
	}

	env.Users[name] = addr
	env.Addrs[addr] = struct{}{}

	return nil
}

// RemoveUser removes a user from the Environment with a given username.
func (env *Environment) RemoveUser(username string) error {
	addr, ok := env.Users[username]
	if !ok {
		return ErrUserNotFound
	}

	delete(env.Users, username)
	delete(env.Addrs, addr)

	// If the user is assigned as default sender, unset
	if env.Sender == username {
		env.Sender = ""
	}
	// If the user is assigned as default receiver, unset
	if env.Receiver == username {
		env.Receiver = ""
	}

	// Delete all entries prefixed with user's address
	if err := env.database.PrefixDelete(db.AddressPrefix(env.ID, addr)); err != nil {
		return err
	}

	return nil
}

// RemoveAllUsers removes all users from the Environment
func (env *Environment) RemoveAllUsers() error {
	// Iterate over the users and remove each one
	for name := range env.Users {
		if err := env.RemoveUser(name); err != nil {
			return err
		}
	}

	// Reset the user registry
	env.Users = make(map[string]identifiers.Address)

	return nil
}

// SetDefaultSender sets a user as sender with a given username.
func (env *Environment) SetDefaultSender(username string) error {
	// Check if user with the given username exists in the inventory
	if !env.UserExists(username) {
		return ErrUserNotFound
	}

	// Check if sender has already been configured
	if env.Sender != "" {
		return ErrSenderAlreadyConf
	}

	// Update the environment
	env.Sender = username

	return nil
}

// SetDefaultReceiver sets a user as receiver with a given username.
func (env *Environment) SetDefaultReceiver(username string) error {
	// Check if user with the given username exists in the inventory
	if !env.UserExists(username) {
		return ErrUserNotFound
	}

	// Check if receiver has already been configured
	if env.Receiver != "" {
		return ErrReceiverAlreadyConf
	}

	// Update the environment
	env.Receiver = username

	return nil
}
