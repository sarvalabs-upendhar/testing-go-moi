package cmds

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
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

	return func(env *Environment) string {
		// Find the logic from the inventory
		logic, exists := env.inventory.FindLogic(name)
		if !exists {
			return fmt.Sprintf("logic '%v' does not exist", name)
		}

		// Get the callsite from the logic, error if not found
		callsite, ok := logic.Logic.GetCallsite(site)
		if !ok {
			return fmt.Sprintf("logic '%v' does not have callsite '%v'", name, site)
		}

		// Perform deploy gating
		// Only allow to deploy, if logic is not ready.
		// Only allow to invoke, if logic is ready.
		if kind == engineio.InvokableCallsite && !logic.Ready {
			return fmt.Sprintf("logic '%v' is not ready for invoke. deploy to initialize persistent state", name)
		} else if kind == engineio.DeployerCallsite && logic.Ready {
			return fmt.Sprintf("logic '%v' is already deployed", name)
		}

		// Check that call kind matches for the callsite
		if callsite.Kind != kind {
			return fmt.Sprintf("callsite '%v' is not a %v", callsite, kind)
		}

		// Obtain the runtime for the logic engine in the header
		runtime, ok := engineio.FetchEngineRuntime(logic.Logic.Engine())
		if !ok {
			return "failed to get runtime for logic"
		}

		// Generate the call encoder for the callsite
		encoder, err := runtime.GetCallEncoder(callsite, logic.Logic)
		if err != nil {
			return fmt.Sprintf("failed to generate call encoder for callsite '%v'", callsite)
		}

		calldata, err := formatArguments(env, args, encoder)
		if err != nil {
			return err.Error()
		}

		// Spawn an engine for the runtime
		engine, err := runtime.SpawnEngine(
			env.inventory.Config.BaseFuel, logic.Logic,
			logic.State.ContextDriver(logic.Logic.ID), env,
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
		senderContext := sender.State.ContextDriver(logic.Logic.ID)

		// Generate an interaction from the kind, callsite, calldata and manifest
		ixn := LogicInteraction{
			kind: func() common.IxType {
				switch kind {
				case engineio.DeployerCallsite:
					return common.IxLogicDeploy
				case engineio.InvokableCallsite:
					return common.IxLogicInvoke
				default:
					panic("unhandled logic call case")
				}
			}(),
			price: new(big.Int).SetUint64(core.LabFuelPrice),
			limit: env.inventory.Config.BaseFuel,
			site:  site,
			call:  calldata,
		}

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

type LogicInteraction struct {
	kind  common.IxType
	price *big.Int
	limit uint64
	site  string
	call  []byte
}

func (ixn LogicInteraction) IxnType() engineio.IxnType { return ixn.kind }
func (ixn LogicInteraction) FuelPrice() *big.Int       { return ixn.price }
func (ixn LogicInteraction) FuelLimit() uint64         { return ixn.limit }
func (ixn LogicInteraction) Callsite() string          { return ixn.site }
func (ixn LogicInteraction) Calldata() []byte          { return ixn.call }

// formatResults formats an engineio.CallResult object into a string.
// It accepts a CallEncoder object to decode any outputs returned with the result
func formatResult(env *Environment, result engineio.CallResult, encoder engineio.CallEncoder) string {
	var str strings.Builder

	if !result.Ok() {
		str.WriteString(fmt.Sprintf("Execution Failed! [%v FUEL]", result.Fuel()))
		str.WriteString(fmt.Sprintf("\nError Data: %#x", result.Error()))

		return str.String()
	}

	str.WriteString(fmt.Sprintf("Execution Complete! [%v FUEL]", result.Fuel()))

	outputs, err := encoder.DecodeOutputs(result.Outputs())
	if err != nil {
		str.WriteString("\nerror: failed to decode execution outputs")

		return str.String()
	}

	if len(outputs) == 0 {
		return str.String()
	}

	str.WriteString("\nExecution Outputs ||| ")

	for name, object := range outputs {
		str.WriteString(fmt.Sprintf("%v: %v ", name, env.format(object)))
	}

	return str.String()
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

func parseArguments(args string) (map[string]any, error) {
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
