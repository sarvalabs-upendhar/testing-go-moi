package compute

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/state"
)

// StateManager describes a state management interface
type StateManager interface {
	// Revert must revert the state of an address to given state.Object
	Revert(*state.Object) error
	// IsAccountRegistered must return whether an address has an existing, registered account
	IsAccountRegistered(common.Address) (bool, error)

	// GetDirtyObject must retrieve the state.Object for a given types.Address
	GetDirtyObject(common.Address) (*state.Object, error)
	// CreateDirtyObject must generate a new dirty state.Object for the given types.Address
	CreateDirtyObject(common.Address, common.AccountType) *state.Object
	// GetLatestStateObject must return the latest state.Object for the given types.Address
	GetLatestStateObject(addr common.Address) (*state.Object, error)
}

// loadStateObjects loads the state.Object for the accounts of the given interaction into the given
// object and snapshot object maps. State validation is enabled with the given stateManager interface.
// The snapshot map is only updated if the withSnapshot argument is true.
//
// If the receiver is not a registered account, this function will also retrieve
// the genesis state object and create a state object for the new account.
func loadStateObjects(
	ix *common.Interaction,
	statemgr StateManager,
	objects, snapshots state.ObjectMap,
	withSnapshots bool,
) error {
	// Fetch state object for sender if valid and not already available
	if sender := ix.Sender(); !sender.IsNil() && objects.GetObject(sender) == nil {
		// Retrieve the dirty object for the sender from the state manager
		senderObject, err := statemgr.GetDirtyObject(sender)
		if err != nil {
			return errors.Wrap(err, "state object fetch failed")
		}

		// Add sender state object and its snapshot
		objects.SetObject(sender, senderObject)

		if withSnapshots {
			snapshots.SetObject(sender, senderObject.Copy())
		}
	}

	// Fetch state object for receiver if valid
	if receiver := ix.Receiver(); !receiver.IsNil() {
		var receiverObject *state.Object

		// Check if the receiver address is an already registered account
		accountRegistered, err := statemgr.IsAccountRegistered(ix.Receiver())
		if err != nil {
			return errors.Wrap(err, "state object fetch failed")
		}

		if !accountRegistered {
			// Retrieve the dirty object for genesis (sarga) address
			genesisObject, err := statemgr.GetDirtyObject(common.SargaAddress)
			if err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}

			// Add genesis state object and its snapshot
			objects.SetObject(common.SargaAddress, genesisObject)

			if withSnapshots {
				snapshots.SetObject(common.SargaAddress, genesisObject.Copy())
			}

			// Create a new dirty state object for the account
			receiverObject = statemgr.CreateDirtyObject(receiver, common.AccTypeFromIxType(ix.Type()))
		} else {
			// Retrieve the dirty object for the receiver from the state manager
			if receiverObject, err = statemgr.GetDirtyObject(receiver); err != nil {
				return errors.Wrap(err, "state object fetch failed")
			}
		}

		// Add receiver state object and its snapshot
		objects.SetObject(receiver, receiverObject)

		if withSnapshots {
			snapshots.SetObject(receiver, receiverObject.Copy())
		}
	}

	return nil
}
