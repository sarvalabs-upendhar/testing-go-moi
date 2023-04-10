package logiclab

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

// CallCommand generates a Command runner to execute a given callsite
// function for a given logic name and some string unparsed arguments.
func CallCommand(kind engineio.CallsiteKind, name, callsite, args string) Command {
	return func(env *Environment) string {
		// Find the logic from the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic '%v' does not exist", name)
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
			env.inventory.BaseFuel, logic.Object,
			logic.CtxState.GenerateLogicContextObject(logic.Object.LogicID()),
			engineio.NewEnvDriver(),
		)
		if err != nil {
			return fmt.Sprintf("failed to bootstrap engine: %v", err)
		}

		// Execute the function
		ixn := engineio.NewIxnObject(kind.IxnType(), callsite, calldata)
		result := engine.Call(context.Background(), ixn, nil)

		return formatResult(result, encoder)
	}
}

func formatArguments(env *Environment, args string, encoder engineio.CallEncoder) (polo.Document, error) {
	// Check if args begins with 0x -> Assume raw calldata provided instead of keyed parameters
	if strings.HasPrefix(args, "0x") {
		// Decode hex string into bytes
		argdata, err := hex.DecodeString(strings.TrimPrefix(args, "0x"))
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse calldata")
		}

		// Decode bytes into a polo.Document
		calldata := make(polo.Document)
		if err = polo.Depolorize(&calldata, argdata); err != nil {
			return nil, errors.Wrap(err, "failed parse calldata into doc")
		}

		return calldata, nil
	}

	// Parse the input arguments into an object map
	arguments, err := parseArguments(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse call arguments")
	}

	// Search the arguments for memory variables
	for key, arg := range arguments {
		// If arg is a memory variable, attempt to retrieve from the memory and
		// update the argument value with the resolved value, otherwise error
		if memvar, ok := arg.(MemoryVar); ok {
			value, exists := env.memory[string(memvar)]
			if !exists {
				return nil, errors.Errorf("memory variable '%v' not found", key)
			}

			arguments[key] = value
		}
	}

	// Encode the parsed arguments into a calldata object
	calldata, err := encoder.EncodeInputs(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode calldata")
	}

	return calldata, nil
}

// formatResults formats an engineio.CallResult object into a string.
// It accepts a CallEncoder object to decode any outputs returned with the result
func formatResult(result *engineio.CallResult, encoder engineio.CallEncoder) string {
	var str strings.Builder

	if !result.Ok() {
		str.WriteString(fmt.Sprintf("Execution Failed! [%v FUEL][Error Code: %v]", result.Fuel, result.ErrCode))
		str.WriteString(fmt.Sprintf("\nError Message: %v", result.ErrMessage))

		return str.String()
	}

	str.WriteString(fmt.Sprintf("Execution Complete! [%v FUEL]", result.Fuel))

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
		str.WriteString(fmt.Sprintf("%v: %v ", name, object))
	}

	return str.String()
}
