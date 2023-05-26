package logiclab

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// LogicCallCommand generates a Command runner to execute a given callsite
// function for a given logic name and some string unparsed arguments.
func LogicCallCommand(kind engineio.CallsiteKind, name, callsite, args string) Command {
	return func(env *Environment) string {
		// Find the logic from the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic '%v' does not exist", name)
		}

		// Perform deploy gating
		// Only allow to deploy, if logic is not ready.
		// Only allow to invoke, if logic is ready.
		if kind == engineio.InvokableCallsite && !logic.Ready {
			return fmt.Sprintf("logic '%v' is not ready for invoke. deploy to initialize persistent state", name)
		} else if kind == engineio.DeployerCallsite && logic.Ready {
			return fmt.Sprintf("logic '%v' is already deployed", name)
		}

		// Get the callsite from the logic, error if not found
		site, ok := logic.Object.GetCallsite(callsite)
		if !ok {
			return fmt.Sprintf("logic '%v' does not have callsite '%v'", name, callsite)
		}

		// Check that call kind matches for the callsite
		if site.Kind != kind {
			return fmt.Sprintf("callsite '%v' is not a %v", callsite, kind)
		}

		// Obtain the runtime for the logic engine in the header
		runtime, ok := engineio.FetchEngineRuntime(logic.Object.Engine())
		if !ok {
			return "failed to get runtime for logic"
		}

		// Generate the call encoder for the callsite
		encoder, err := runtime.GetCallEncoder(site, logic.Object)
		if err != nil {
			return fmt.Sprintf("failed to generate call encoder for callsite '%v'", callsite)
		}

		calldata, err := formatArguments(env, args, encoder)
		if err != nil {
			return err.Error()
		}

		// Spawn an engine for the runtime
		engine, err := runtime.SpawnEngine(
			env.inventory.Config.BaseFuel, logic.Object,
			logic.CtxState.GenerateLogicContextObject(logic.Object.LogicID()),
			engineio.NewEnvDriver(),
		)
		if err != nil {
			return fmt.Sprintf("failed to bootstrap engine: %v", err)
		}

		// Fetch the designated sender
		sender, err := fetchDesignatedSenderState(env)
		if err != nil {
			return fmt.Sprintf("failed to fetch state for sender: %v", err)
		}

		// Generate the context object for the sender
		senderContext := sender.CtxState.GenerateLogicContextObject(logic.Object.LogicID())
		// Generate the interaction object
		ixn := engineio.NewIxnObject(kind.IxnType(), callsite, calldata)

		// Execute the function
		result, err := engine.Call(context.Background(), ixn, senderContext)
		if err != nil {
			return fmt.Sprintf("failed to perform logic call: %v", err)
		}

		if kind == engineio.DeployerCallsite && result.Ok() {
			logic.Ready = true
		}

		return formatResult(env, result, encoder)
	}
}

func formatArguments(env *Environment, args string, encoder engineio.CallEncoder) ([]byte, error) {
	// Check if args begins with 0x -> Assume raw calldata provided instead of keyed parameters
	if strings.HasPrefix(args, "0x") {
		// Decode hex string into bytes
		argdata, err := hex.DecodeString(strings.TrimPrefix(args, "0x"))
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse calldata")
		}

		return argdata, nil
	}

	// Parse the input arguments into an object map
	arguments, err := parseArguments(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse call arguments")
	}

	// Encode the parsed arguments into a calldata object
	calldata, err := encoder.EncodeInputs(arguments, env)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode calldata")
	}

	return calldata, nil
}

// formatResults formats an engineio.CallResult object into a string.
// It accepts a CallEncoder object to decode any outputs returned with the result
func formatResult(env *Environment, result *engineio.CallResult, encoder engineio.CallEncoder) string {
	var str strings.Builder

	if !result.Ok() {
		str.WriteString(fmt.Sprintf("Execution Failed! [%v FUEL]", result.Consumed))
		str.WriteString(fmt.Sprintf("\nError Data: %#x", result.Error))

		return str.String()
	}

	str.WriteString(fmt.Sprintf("Execution Complete! [%v FUEL]", result.Consumed))

	outputs, err := encoder.DecodeOutputs(result.Outputs)
	if err != nil {
		str.WriteString("\nerror: failed to decode execution outputs")

		return str.String()
	}

	if len(outputs) == 0 {
		return str.String()
	}

	str.WriteString("\nExecution Outputs ||| ")

	for name, object := range outputs {
		str.WriteString(fmt.Sprintf("%v: %v ", name, env.formatValue(object)))
	}

	return str.String()
}

func fetchDesignatedSenderState(env *Environment) (*ParticipantState, error) {
	if env.inventory.Sender == "" {
		return nil, ErrNoDesignatedSender
	}

	return fetchSenderState(env, env.inventory.Sender)
}

func fetchSenderState(env *Environment, username string) (*ParticipantState, error) {
	// Find the participant in the inventory
	participant, exists := env.inventory.FindParticipant(username)
	if !exists {
		return nil, errors.Errorf("participant '%v' not found", username)
	}

	return participant, nil
}
