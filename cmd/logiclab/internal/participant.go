package internal

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/common"
)

// ParticipantState is a container for all contents
// related to a Participant entity in the LogicLab
type ParticipantState struct {
	Username string
	Address  common.Address
	CtxState *StateObject
}

// NewParticipantState generates a new ParticipantState for a given
// username. The address of the participant is generated randomly
func NewParticipantState(name string) *ParticipantState {
	// Generate a random address
	addr := randomAddress()
	// Generate and return a ParticipantState with the new
	// address a new StateObject generated from that address
	return &ParticipantState{
		Username: name,
		Address:  addr,
		CtxState: NewStateObject(addr),
	}
}

// String returns a string representation of the ParticipantState.
// Implements the Stringer interface for ParticipantState.
func (participant ParticipantState) String() string {
	return fmt.Sprintf("%v\t[%v]", participant.Username, participant.Address)
}

// ParticipantRegisterCommand generates a Command runner
// to register a new Participant with the given username
func ParticipantRegisterCommand(username string) Command {
	return func(env *Environment) string {
		// Check if a participant with username already exists
		if exists := env.inventory.ParticipantExists(username); exists {
			return fmt.Sprintf("participant %v already exists", username)
		}

		// Generate a new Participant state for the username
		participant := NewParticipantState(username)
		// Add the participant to the inventory
		env.inventory.AddParticipant(participant)

		return fmt.Sprintf("participant '%v' created with address '%v'", username, participant.Address)
	}
}

// ParticipantDeleteCommand generates a Command runner to
// delete an existing Participant with the given username
func ParticipantDeleteCommand(username string) Command {
	return func(env *Environment) string {
		// Check if a participant with username exists
		if exists := env.inventory.ParticipantExists(username); !exists {
			return fmt.Sprintf("participant %v does not exist", username)
		}

		// Remove the participant from the inventory
		env.inventory.RemoveParticipant(username)

		return fmt.Sprintf("participant '%v' removed", username)
	}
}

// ParticipantInspectCommand generates a Command runner to print
// the details of a specific participant with a given username
func ParticipantInspectCommand(username string) Command {
	return func(env *Environment) string {
		// Find the participant in the inventory
		participant, exists := env.inventory.FindParticipant(username)
		if !exists {
			return fmt.Sprintf("participant '%v' does not exist", username)
		}

		return participant.String()
	}
}

// ParticipantListCommand generates a Command runner
// to print details of all registered participants
func ParticipantListCommand() Command {
	return func(env *Environment) string {
		var (
			idx  = 1
			list strings.Builder
		)

		for username, address := range env.inventory.Participants {
			list.WriteString(fmt.Sprintf("%v] %v [%v]", idx, username, address))

			if idx++; idx <= len(env.inventory.Participants) {
				list.WriteString("\n")
			}
		}

		if idx == 1 {
			list.WriteString("no participants found")
		}

		return list.String()
	}
}

var (
	ErrNoDesignatedSender   = errors.New("no designated sender")
	ErrNoDesignatedReceiver = errors.New("no designated receiver")
)

// DesignatedSenderCommand generates a Command runner
// to print the current designated sender participant
func DesignatedSenderCommand() Command {
	return func(env *Environment) string {
		if name := env.inventory.Sender; name != "" {
			return fmt.Sprintf("'%v' is the designated sender", name)
		}

		return ErrNoDesignatedSender.Error()
	}
}

// DesignatedReceiverCommand generates a Command runner
// to print the current designated receiver participant
func DesignatedReceiverCommand() Command {
	return func(env *Environment) string {
		if name := env.inventory.Receiver; name != "" {
			return fmt.Sprintf("'%v' is the designated receiver", name)
		}

		return ErrNoDesignatedReceiver.Error()
	}
}

// randomAddress generates a random types.Address.
func randomAddress() common.Address {
	address := make([]byte, 32)
	_, _ = rand.Read(address)

	return common.BytesToAddress(address)
}
