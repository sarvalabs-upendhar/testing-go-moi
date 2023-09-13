package core

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
)

// UserAccount is a container for all contents
// related to a participant entity in the LogicLab
type UserAccount struct {
	Name  string
	Addr  common.Address
	State *AccountState
}

// NewUserAccount generates a new UserAccount for a given
// username. The address of the user is generated randomly
func NewUserAccount(name string, address common.Address) *UserAccount {
	// Generate a random address if not provided
	if address == common.NilAddress {
		address = RandomAddress()
	}

	// Generate and return a UserAccount with the name, address
	// and a new AccountState generated from that address
	return &UserAccount{
		Name:  name,
		Addr:  address,
		State: NewAccountState(address),
	}
}

// String returns a string representation of the UserAccount.
// Implements the Stringer interface for UserAccount.
func (user UserAccount) String() string {
	return fmt.Sprintf("%v\t[@>%v<@]", user.Name, user.Addr)
}

// AddUser adds a core.UserAccount object to the Inventory.
// The user is indexed by their username.
func (inventory *Inventory) AddUser(user *UserAccount) {
	inventory.ucache[user.Name] = user
	inventory.Users[user.Name] = user.Addr
}

// RemoveUser removes a user from the Inventory with a given username.
func (inventory *Inventory) RemoveUser(username string) {
	delete(inventory.ucache, username)
	delete(inventory.Users, username)
	_ = deleteFile(userFilename(inventory.labdir, username))
}

// UserExists returns whether a user with the given name exists in the Inventory
func (inventory *Inventory) UserExists(username string) bool {
	_, exists := inventory.Users[username]

	return exists
}

// FindUser attempts to find a user and return their core.UserAccount for a given username.
// Fails if no such user is tracked by the Inventory or if the file for a known user is missing.
func (inventory *Inventory) FindUser(username string) (*UserAccount, bool) {
	// Check if user exists
	if exists := inventory.UserExists(username); !exists {
		return nil, false
	}

	// Check if known user is available in the cache
	if user, cached := inventory.ucache[username]; cached {
		return user, true
	}

	// Generate filename for user
	filename := userFilename(inventory.labdir, username)
	// Load participant from file
	user := new(UserAccount)
	if err := loadFile(filename, user); err != nil {
		// If the file for a known participant
		// is missing, forget the participant
		if errors.Is(err, ErrMissingFile) {
			delete(inventory.Users, username)
		}

		return nil, false
	}

	// Store the retrieved UserAccount to the inventory cache
	inventory.ucache[username] = user

	return user, true
}
