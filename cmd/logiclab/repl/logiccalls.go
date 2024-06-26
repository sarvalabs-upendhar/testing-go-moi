package repl

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

func HelpDeploy() string {
	//nolint:lll
	return `
The @>deploy<@ command can be used to make a logic call with the interaction type IxLogicDeploy.
The [@>logics<@] name must already exist and have the given callsite which must be a deployer.

A deployer can only be called if the logic is marked as not ready (defaults to false if the compiled logic has a persistent state).
Once, deployed, it is marked as @>ready<@ and cannot be the target of the deploy command again but can be used with [@>invoke<@].

The calldata for the logic call can be provided as a series of key value pairs (call arguments) which would 
follow the [@>argument<@] value rules and be encoded with the runtime CallEncoder for the input callsite.
Alternately, the calldata can be directly provided as raw bytes which POLO doc-encoded in its hex string 
form. This raw format of the calldata can be generated using the [@>callencode<@] command.

usage:
@>deploy [name].[callsite]([call arguments])<@ 
@>deploy [name].[callsite]([raw calldata])<@ 

examples:
>> set addr1 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34
>> deploy Ledger.Seeder!(name: "MOI-Token", symbol: "MOI", supply: 100000000, seeder: addr1)
Execution Complete! [150 FUEL]

>> deploy Ledger.Seeder!(0x0def010645e601c502d606b5078608e5086e616d65064d4f492d546f6b656e73656564657206f6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34737570706c790305f5e10073796d626f6c064d4f49)
Execution Complete! [150 FUEL]
`
}

func HelpInvoke() string {
	//nolint:lll
	return `
The @>invoke<@ command can be used to make a logic call with the interaction type IxLogicInvoke.
The [@>logics] name must already exist and have the given callsite which must be an invokable.

An invokable can only be called if the logic is marked as ready (defaults to false if the compiled logic has a persistent state).
It is required to [@>deploy<@] an unready logic before it can be the target of an invoke command.

The calldata for the logic call can be provided as a series of key value pairs (call arguments) which would 
follow the [@>argument<@] value rules and be encoded with the runtime CallEncoder for the input callsite.
Alternately, the calldata can be directly provided as raw bytes which POLO doc-encoded in its hex string 
form. This raw format of the calldata can be generated using the [@>callencode<@] command.

usage:
@>invoke [name].[callsite]([call arguments])<@ 
@>invoke [name].[callsite]([raw calldata])<@ 

examples:
>> invoke Ledger.BalanceOf(addr: 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
Execution Complete! [90 FUEL]
Execution Outputs ||| balance: 100000000

>> invoke Ledger.BalanceOf(0x0d2f06456164647206f6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
Execution Complete! [90 FUEL]
Execution Outputs ||| balance: 100000000
`
}

func parseLogicCall(parser *symbolizer.Parser, kind engineio.CallsiteKind) Command {
	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing logic identifier")
	}

	name := parser.Cursor().Literal

	if !parser.ExpectPeek(symbolizer.TokenKind('.')) {
		return InvalidCommandError("missing . after logic name")
	}

	if !parser.ExpectPeek(symbolizer.TokenIdent) {
		return InvalidCommandError("missing logic callsite")
	}

	site := parser.Cursor().Literal

	if parser.ExpectPeek(symbolizer.TokenKind('!')) || parser.ExpectPeek(symbolizer.TokenKind('$')) {
		site += parser.Cursor().Literal
	}

	parser.Advance()

	args, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return InvalidCommandErrorf("malformed args: %v", err)
	}

	return func(repl *Repl) string {
		// Find the logic from the inventory
		logic, err := repl.env.FetchLogic(name)
		if err != nil {
			return fmt.Sprintf("logic '%v' does not exist: %v", name, err)
		}

		// Get the callsite from the logic, error if not found
		callsite, ok := logic.Object.GetCallsite(site)
		if !ok {
			return fmt.Sprintf("logic '%v' does not have callsite '%v'", name, site)
		}

		// Perform deploy gating
		// Only allow to deploy, if logic is not ready.
		// Only allow to invoke, if logic is ready.
		if kind == engineio.CallsiteInvoke && !logic.Ready {
			return fmt.Sprintf("logic '%v' is not ready for invoke. deploy to initialize persistent state", name)
		} else if kind == engineio.CallsiteDeploy && logic.Ready {
			return fmt.Sprintf("logic '%v' is already deployed", name)
		}

		// Check that call kind matches for the callsite
		if callsite.Kind != kind {
			return fmt.Sprintf("callsite '%v' is not a %v", callsite, kind)
		}

		// Obtain the runtime for the logic engine in the header
		engine, ok := engineio.FetchEngine(logic.Object.Engine())
		if !ok {
			return "failed to get runtime for logic"
		}

		// Generate the call encoder for the callsite
		encoder, err := engine.GetCallEncoderFromLogicDriver(logic.Object, callsite)
		if err != nil {
			return fmt.Sprintf("failed to generate call encoder for callsite '%v'", callsite)
		}

		calldata, err := repl.formatArguments(args, encoder)
		if err != nil {
			return err.Error()
		}

		logicID := logic.Object.ID
		eventstream := compute.NewEventStream(logicID)

		// Spawn an engine for the runtime
		instance, err := engine.SpawnInstance(
			logic.Object,
			repl.env.CallFuel,
			core.NewStorageDriver(repl.env.ID, repl.lab.Database, logicID.Address(), logicID),
			repl.lab,
			eventstream,
		)
		if err != nil {
			return fmt.Sprintf("failed to bootstrap engine: %v", err)
		}

		// Fetch the designated sender
		sender, err := repl.GetDesignatedSender()
		if err != nil {
			return fmt.Sprintf("failed to fetch state for sender: %v", err)
		}

		// Generate the context object for the sender
		senderContext := core.NewStorageDriver(repl.env.ID, repl.lab.Database, sender, logicID)

		// Generate an interaction from the kind, callsite, calldata and manifest
		ixn := core.LogicInteraction{
			Kind: func() common.IxType {
				switch kind {
				case engineio.CallsiteDeploy:
					return common.IxLogicDeploy
				case engineio.CallsiteInvoke:
					return common.IxLogicInvoke
				default:
					panic("unhandled logic call case")
				}
			}(),
			Nonce: repl.env.Nonce,
			Price: new(big.Int).SetUint64(core.LabFuelPrice),
			Limit: repl.env.CallFuel,
			Site:  site,
			Call:  calldata,
		}

		// Execute the function
		result, err := instance.Call(context.Background(), ixn, senderContext)
		if err != nil {
			return fmt.Sprintf("failed to perform logic call: %v", err)
		}

		if kind == engineio.CallsiteDeploy && result.Ok() {
			logic.Ready = true
		}

		ixnHash, err := ixn.Hash()
		if err != nil {
			return fmt.Sprintf("failed to hash logic call: %v", err)
		}

		return repl.formatResult(ixnHash, result, eventstream, encoder)
	}
}

// formatResult FormatResults formats an engineio.CallResult object into a string.
// It accepts a CallEncoder object to decode any outputs returned with the result
func (repl *Repl) formatResult(
	hash common.Hash,
	result engineio.CallResult,
	stream *compute.EventStream,
	encoder engineio.CallEncoder,
) string {
	var str strings.Builder

	if !result.Ok() {
		str.WriteString(fmt.Sprintf("Execution Failed! [%v FUEL]\n", result.Fuel()))
		str.WriteString(fmt.Sprintf("Error Data: %#x\n", result.Error()))

		return str.String()
	}

	str.WriteString(fmt.Sprintf("Execution Complete! [%v FUEL]\n", result.Fuel()))
	str.WriteString(fmt.Sprintf("Interaction Hash: %v\n", hash.String()))

	if len(result.Outputs()) == 0 {
		return str.String()
	}

	outputs, err := encoder.DecodeOutputs(result.Outputs())
	if err != nil {
		str.WriteString("error: failed to decode execution outputs\n")

		return str.String()
	}

	str.WriteString("Execution Outputs |||\n")

	for name, object := range outputs {
		formatted := repl.FormatValue(object)
		str.WriteString(fmt.Sprintf("%v: %v\n", name, formatted))
	}

	if stream.Count() == 0 {
		return str.String()
	}

	str.WriteString("Execution Logs:\n")

	for event := range stream.Iterate() {
		str.WriteString(fmt.Sprintf(">> [%#x] %#x", event.Address, event.Data))
	}

	return str.String()
}

func (repl *Repl) formatArguments(args string, encoder engineio.CallEncoder) ([]byte, error) {
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
	arguments, err := argparse(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse call arguments")
	}

	// Encode the parsed arguments into a calldata object
	calldata, err := encoder.EncodeInputs(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode calldata")
	}

	return calldata, nil
}

func argparse(args string) (map[string]any, error) {
	arguments := make(map[string]any)

	if args == "" {
		return arguments, nil
	}

	keyedArgs, detection, err := parseKeyedValues(args, symbolizer.TokenKind(','))
	if err != nil {
		return nil, errors.Wrap(err, "malformed arguments")
	}

	if detection != symbolizer.TokenIdent {
		return nil, errors.Errorf("malformed arguments: missing identifier keys")
	}

	for key, arg := range keyedArgs {
		arguments[key.(string)] = arg //nolint:forcetypeassert
	}

	return arguments, nil
}
