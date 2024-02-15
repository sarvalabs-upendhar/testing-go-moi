package compute

import (
	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// StateManager describes a state management interface
type StateManager interface {
	// IsAccountRegistered must return whether an address has an existing, registered account
	IsAccountRegistered(identifiers.Address) (bool, error)
	// CreateStateObject must create a new state.Object for the given address and account type
	CreateStateObject(identifiers.Address, common.AccountType) *state.Object

	// GetDirtyObject must retrieve the state.Object for a given types.Address
	GetDirtyObject(identifiers.Address) (*state.Object, error)
	// CreateDirtyObject must generate a new dirty state.Object for the given types.Address
	CreateDirtyObject(identifiers.Address, common.AccountType) *state.Object

	// GetLatestStateObject must return the latest state.Object for the given types.Address
	GetLatestStateObject(addr identifiers.Address) (*state.Object, error)
	// GetStateObjectByHash must return the latest state.Object for the given hash
	GetStateObjectByHash(addr identifiers.Address, hash common.Hash) (*state.Object, error)

	// UpdateStateObjects must update the existing state objects
	UpdateStateObjects(objs state.ObjectMap) error

	// Cleanup deletes the dirty state objects
	Cleanup(addr identifiers.Address)
}

func FetchIxStateObjects(
	source StateManager, ix *common.Interaction,
	hashes map[identifiers.Address]common.Hash,
) (
	state.ObjectMap, error,
) {
	// Create a map of state objects
	objects := make(state.ObjectMap)

	// Fetch state object for sender if valid
	if sender := ix.Sender(); !sender.IsNil() {
		var (
			err          error
			senderObject *state.Object
		)

		if stateHash, ok := hashes[sender]; !ok {
			// Retrieve the dirty object for the sender from the state manager
			senderObject, err = source.GetLatestStateObject(sender)
			if err != nil {
				return nil, errors.Wrap(err, "sender state object fetch failed")
			}
		} else {
			senderObject, err = source.GetStateObjectByHash(sender, stateHash)
			if err != nil {
				return nil, errors.Wrap(err, "failed to fetch sender state object by hash")
			}
		}

		// Add sender state object
		objects.SetObject(sender, senderObject)
	}

	// Fetch state object for receiver if valid
	if receiver := ix.Receiver(); !receiver.IsNil() {
		var receiverObject *state.Object

		// Check if the receiver address is an already registered account
		accountRegistered, err := source.IsAccountRegistered(ix.Receiver())
		if err != nil {
			return nil, errors.Wrap(err, "state object fetch failed")
		}

		if !accountRegistered {
			var genesisObject *state.Object

			if stateHash, ok := hashes[common.SargaAddress]; !ok {
				// Retrieve the dirty object for genesis (sarga) address
				genesisObject, err = source.GetLatestStateObject(common.SargaAddress)
				if err != nil {
					return nil, errors.Wrap(err, "genesis state object fetch failed")
				}
			} else {
				genesisObject, err = source.GetStateObjectByHash(common.SargaAddress, stateHash)
				if err != nil {
					return nil, errors.Wrap(err, "failed to fetch genesis state object by hash")
				}
			}

			// Add genesis state object
			objects.SetObject(common.SargaAddress, genesisObject)
			// Create a dummy state object for the receiver
			receiverObject = source.CreateStateObject(receiver, common.AccTypeFromIxType(ix.Type()))
		} else {
			if stateHash, ok := hashes[receiver]; !ok {
				// Retrieve the dirty object for the receiver from the state manager
				if receiverObject, err = source.GetLatestStateObject(receiver); err != nil {
					return nil, errors.Wrap(err, "receiver state object fetch failed")
				}
			} else {
				receiverObject, err = source.GetStateObjectByHash(receiver, stateHash)
				if err != nil {
					return nil, errors.Wrap(err, "failed to fetch receiver state object by hash")
				}
			}
		}

		// Add receiver state object and its snapshot
		objects.SetObject(receiver, receiverObject)
	}

	return objects, nil
}
