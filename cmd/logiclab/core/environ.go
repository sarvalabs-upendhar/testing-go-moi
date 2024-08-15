package core

import (
	"fmt"

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

	eventsHead uint64
	eventsSize uint64

	ID    string
	Nonce uint64

	Addrs  map[identifiers.Address]struct{}
	Users  map[string]identifiers.Address
	Logics map[string]LogicMetadata

	Sender   string
	CallFuel uint64

	Config ReplConfig
}

type ReplConfig struct {
	HexBigInt bool
	HexBytes  bool
}

func NewEnvironment(name string, database db.Database) *Environment {
	return &Environment{
		database:   database,
		lcache:     make(map[string]*Logic),
		eventsHead: 0,
		eventsSize: 0,

		ID:       name,
		Nonce:    0,
		Addrs:    map[identifiers.Address]struct{}{},
		Users:    make(map[string]identifiers.Address),
		Logics:   make(map[string]LogicMetadata),
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

	value, err := polo.Polorize(env.eventsHead)
	if err != nil {
		return nil, err
	}

	if err := env.database.Set(db.EventHeadKey(env.ID), value); err != nil {
		return nil, err
	}

	value, err = polo.Polorize(env.eventsSize)
	if err != nil {
		return nil, err
	}

	if err := env.database.Set(db.EventSizeKey(env.ID), value); err != nil {
		return nil, err
	}

	return encoded, nil
}

func (env *Environment) Decode(encoded []byte) error {
	envdb := env.database
	if envdb == nil {
		return errors.New("database unavailable")
	}

	if err := polo.Depolorize(env, encoded); err != nil {
		return errors.Wrap(err, "failed to depolorize environment")
	}

	env.database = envdb

	headValue, err := env.database.Get(db.EventHeadKey(env.ID))
	if err != nil {
		return fmt.Errorf("failed to get event head: %w", err)
	}

	// Get and decode size
	sizeValue, err := env.database.Get(db.EventSizeKey(env.ID))
	if err != nil {
		return fmt.Errorf("failed to get event size: %w", err)
	}

	var head, size uint64

	// Decode the head value
	err = polo.Depolorize(&head, headValue)
	if err != nil {
		return err
	}

	// Decode the size value
	err = polo.Depolorize(&size, sizeValue)
	if err != nil {
		return err
	}

	// Initialize the logic cache
	env.lcache = make(map[string]*Logic)
	env.eventsSize = size
	env.eventsHead = head

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
		addr = env.generateUniqueRandomAddress()
	} else if env.AddrExists(addr) {
		return ErrAddrAlreadyExists
	}

	env.Users[name] = addr
	env.Addrs[addr] = struct{}{}

	account := &Account{
		Kind: UserAccount,
		Name: name,
		Data: nil,
	}

	rawAccount, err := account.Encode()
	if err != nil {
		return err
	}

	if err = env.database.Set(db.AccountKey(env.ID, addr), rawAccount); err != nil {
		return err
	}

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

	// Delete all entries prefixed with user's address
	if err := env.database.PrefixDelete(db.AccountPrefix(env.ID, addr)); err != nil {
		return err
	}

	return nil
}

// PurgeUsers removes all users from the Environment
func (env *Environment) PurgeUsers() error {
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

// LookupAccount returns the account details associated with the given address
func (env *Environment) LookupAccount(addr identifiers.Address) (AccountKind, string) {
	// Check if the address exists in the environment
	if !env.AddrExists(addr) {
		return UnknownAccKind, ""
	}

	// Get the raw account details from the db
	rawAccount, err := env.database.Get(db.AccountKey(env.ID, addr))
	if err != nil {
		return UnknownAccKind, err.Error()
	}

	account := new(Account)
	// Convert into Account type
	err = polo.Depolorize(account, rawAccount)
	if err != nil {
		return UnknownAccKind, err.Error()
	}

	return account.Kind, account.Name
}

func (env *Environment) Enlisted(addr identifiers.Address, logic identifiers.LogicID) bool {
	// Check if the address exists in the environment
	if !env.AddrExists(addr) {
		return false
	}

	// Get the raw account details from the db
	ok, err := env.database.Has(db.LogicStoragePrefix(env.ID, addr, logic))
	if err != nil {
		return false
	}

	return ok
}

func (env *Environment) Enlist(addr identifiers.Address, logic identifiers.LogicID) error {
	// Check if the address exists in the environment
	if !env.AddrExists(addr) {
		return errors.New("user already enlisted")
	}

	// Get the raw account details from the db
	if err := env.database.Set(db.LogicStoragePrefix(env.ID, addr, logic), []byte{2}); err != nil {
		return err
	}

	return nil
}

// IncrementNonce increases the nonce by 1.
func (env *Environment) IncrementNonce() {
	env.Nonce += 1
}

func (env *Environment) generateUniqueRandomAddress() identifiers.Address {
	addr := identifiers.NewRandomAddress()
	if env.AddrExists(addr) {
		// If the generated address already exists, recursively generate a new one
		return env.generateUniqueRandomAddress()
	}

	return addr
}
