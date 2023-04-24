package logiclab

import (
	"fmt"

	"github.com/manishmeganathan/symbolizer"
)

// Command is a function form that accepts a logiclab Environment,
// performs some operations in it and then returns a string output.
type Command func(env *Environment) string

//nolint:lll
var CommandHelp = "LogicLab Commands Help" + "\n" + replStrike + `
exit - Exit LogicLab REPL
help - Print LogicLab Command Documentation

==== Memory Variables
Memory Variable are session memory values that can be used for holding values that 
can be used during logic calls with the invoke and deploy commands. memory variables
are lost after each session and can be manipulated with the following commands. 
Refer to Argument Values Section for value syntax rules

set [identifier] [value] - Set the value of a memory variable with the identifier
get [identifier]         - Get the value of a memory variable with the identifier

Example:
>> set addr1 0x0fafe52ec42a85db644d5cceba2bb89cf5b0166cc9158211f44ed1e60b06032c
'addr1' has been set to [15 175 229 46 196 42 133 219 100 77 92 206 186 43 184 156 245 176 22 108 201 21 130 17 244 78 209 230 11 6 3 44]

>> get addr1 
'addr1' is set to [15 175 229 46 196 42 133 219 100 77 92 206 186 43 184 156 245 176 22 108 201 21 130 17 244 78 209 230 11 6 3 44]

==== Designated Participants
Designated Participants are specially designated participants for being the default 
Sender and/or Receiver for logic call where applicable. The referenced Participant 
must exist and be registered with the Lab environment (refer to Participants Section).
Designated Participants are saved and available across multiple sessions.

designate [username] as [sender/receiver] - Designate the participant with username as the default sender or receiver
designated [sender/receiver]              - View the designated participant for the default sender or receiver

Example:
>> designate Manish as sender
'Manish' has been designated as the sender

>> designate Rahul as receiver
participant Rahul does not exist

>> designated sender
'Manish' is the designated sender

>> designated receiver
no designated receiver

==== Participants
Participants are registered accounts with LogicLab with a fully available context 
state. They can be used to invoke logic functions or interact with other Participants
through a logic call. They are indexed by a unique username and are saved by default

participant - List all registered participants
participant inspect [username]  - Inspect the contents of the Participant State of given username
participant delete [username]   - Delete a Participant with the given username.
participant register [username] - Register a new Participant with the given username. Address is generated randomly

Examples:
>> participant register Rahul
participant 'Rahul' created with address '0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e'

>> participant
1] Manish [0xf53b821f2155c03592ffff397780ffae644f908aed1ecb8e84a12fa961ed0363]
2] Rahul [0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e]

>> participant inspect Rahul
Rahul [0xb1107436395807a00c0d673134d48956315a0c65af620a95a6ada9470fef276e]

>> participant delete Rahul 
participant 'Rahul' removed

==== Logics
Logics are compiled logic objects that can be executed in their defined engine.
They can be compiled directly from a Manifest file and inspected to view their state.

logic - List all compiled logics

logic inspect [name] - Inspect the contents of the Logic State of given name
logic delete [name]  - Delete a Logic with the given name.
logic compile [name] from manifest([filepath]) - Compile a logic from a Manifest at the given file path. The manifest
encoding is inferred from the file extension (.polo for POLO). The compiled logic is indexed by the given name

Examples:
>> logic compile ERC20 from manifest("./jug/manifests/erc20.json")     
logic 'ERC20' [0800007140e42388a825992f5f07c7711718384b0ef228b36f46511503295e1dc38931] compiled with 100 FUEL

>> logic
1] ERC20 [080000204d61aca8d5562d71ead8162fc9eb6de57bae3ab2cbb5513e61b0eb39ffa11f]
2] Flipper [080000df4824f93ea1ce70f8540840817e4231c2af219bb99b048a5165c6e60f36a599]

>> logic inspect Flipper
==== [ Flipper ] [Address: 0xdf4824f93ea1ce70f8540840817e4231c2af219bb99b048a5165c6e60f36a599]
[Edition: 0] [Logic ID: 080000df4824f93ea1ce70f8540840817e4231c2af219bb99b048a5165c6e60f36a599]
[Engine: PISA] [Manifest: b0e336028515909d12bcfa69d99a048b7988a8b03e61a5d5471f02112d62575c]
[Persistent: true] [Ephemeral: false] [Interactive: false] [Asset Logic: false]

==== Callsites
[1] Flip! [invokable]
[2] Mode [invokable]
====

==== State
03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314: 02
====

>> logic delete ERC20
logic 'ERC20' removed

==== Logic Function Calls
Logic functions can called with several commands depending on the nature of function callsite 
as defined by the logic. The arguments for the call are parsed from rules specified in Argument 
Values Section and encoded by the Call Encoder generated for the callsite. The callsite must be 
valid and match the form of call for the logic call to succeed.

deploy [name] [callsite](calldata) 
invoke [name] [callsite](calldata)

The calldata for the logic call can be provided as a series of key value pairs which will be
encoded with the runtime CallEncoder for the input specification and validated accordingly.
Alternately, the calldata can be directly provided as a POLO Document in its hex string form. 
This calldata can be generated from argument values. (refer to the Callgen Utility Section)

Examples:
>> set addr1 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34
>> deploy ERC20 Seeder!(name: "MOI-Token", symbol: "MOI", supply: 100000000, seeder: addr1)
Execution Complete! [150 FUEL]

>> invoke ERC20 Name()
Execution Complete! [70 FUEL]
Execution Outputs ||| name: MOI-Token

>> invoke ERC20 BalanceOf(addr: 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
Execution Complete! [90 FUEL]
Execution Outputs ||| balance: 100000000

>> invoke ERC20 BalanceOf(0x0d2f06456164647206f6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
Execution Complete! [90 FUEL]
Execution Outputs ||| balance: 100000000

==== Argument Values
Argument Value Rules are used when parsing the argument in logic function calls or when storing
them to the environment session memory. Logic function calls can also use variables from the memory.

Supported types
- Integer (Ex: 100, -934343, 329429352)
- String (Ex: "Hello", "Fahrenheit 451")
- Boolean (Ex: true, True, TRUE, false, False, FALSE)
- Bytes/Address (Ex: 0xf6cd8ee6a29ec442dbbf9c6124dd3aeb833ef58052237d521654740857716b34)
- Lists (Ex: [256, 2345], ["foo", "bar"])
- Mappings (Ex: {"a": 123, "b": 345}, {456: "foo", 123: "bar"}) // value keys
- Objects (Ex: {a: 123, b: 345}, {name: "Darius", age: 45})     // ident keys

==== Slothash Utility
Slothash for accessing storage data can be generated with the slothash command.
Currently it only supports a simple slot hashing by accepting a uint8 slot and 
returning its hash, but this will be extended when PISA's storage layer is complete.

slothash [slot]

Examples:
>> slothash 0 
03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314

==== Callgen Utility
Raw calldata for logic calls can be generated with the callgen command.
Callgen can be performed on objects from the lab memory or directly with an object literal.
The returned calldata is the doc-encoded hex string of the object.

callgen [identifier]
callgen [object]

Examples:
>> set A 500
>> set B "manish"
>> set C {name: A, value: B}
>> callgen C
0x0d5f064576c5016e616d650301f476616c7565066d616e697368

>> callgen {name: A, value: B}
0x0d5f064576c5016e616d650301f476616c7565066d616e697368

==== Manifest Utility
Converting manifests between encoding schemes can be done with the manifest utility.
The given manifest at the filepath is decoded and printed in the encoding of choice.
Returns indented and formatted data for JSON and YAML, and hex string for POLO.

manifest([filepath]) as [encoding]

Example:
>> manifest("./jug/manifests/erc20.json") as JSON
// prints JSON object 

>> manifest("./jug/manifests/erc20.json") as YAML
// prints YAML object

>> manifest("./jug/manifests/erc20.json") as POLO
// prints hex encoded string of POLO bytes
` + replStrike

// ParseCommand parses an input command string into a Command runner
func ParseCommand(cmd string) Command {
	parser := symbolizer.NewParser(cmd,
		symbolizer.IgnoreWhitespaces(),
		symbolizer.Keywords(keywords),
	)

	switch parser.Cursor().Kind {
	case TokenHelp:
		return HelpCommand()

	case TokenDesignate:
		return parseDesignateCommand(parser)
	case TokenDesignated:
		return parseDesignatedCommand(parser)

	case TokenCallAction:
		return parseCallActionCommand(parser)
	case TokenMemoryAction:
		return parseMemoryActionCommand(parser)

	case TokenParticipant:
		return parseParticipantCommand(parser)
	case TokenLogic:
		return parseLogicCommand(parser)
	case TokenManifest:
		return parseManifestCommand(parser)

	case TokenCallgen:
		return parseCallgenCommand(parser)
	case TokenSlothash:
		return parseSlothashCommand(parser)

	case TokenExit:
		return ExitCommand()
	default:
		return InvalidCommandError("")
	}
}

// DesignateCommand generates a Command runner to set
// the designated sender/receiver for all logic calls
func DesignateCommand(actor, name string) Command {
	return func(env *Environment) string {
		if _, exists := env.inventory.Participants[name]; !exists {
			return fmt.Sprintf("participant %v does not exist", name)
		}

		switch actor {
		case "sender":
			env.inventory.Sender = name
		case "receiver":
			env.inventory.Receiver = name
		default:
			return fmt.Sprintf("actor '%v' is not supported", actor)
		}

		return fmt.Sprintf("'%v' has been designated as the %v", name, actor)
	}
}

// DesignatedSenderCommand generates a Command runner
// to print the current designated sender participant
func DesignatedSenderCommand() Command {
	return func(env *Environment) string {
		if name := env.inventory.Sender; name != "" {
			return fmt.Sprintf("'%v' is the designated sender", name)
		}

		return "no designated sender"
	}
}

// DesignatedReceiverCommand generates a Command runner
// to print the current designated receiver participant
func DesignatedReceiverCommand() Command {
	return func(env *Environment) string {
		if name := env.inventory.Receiver; name != "" {
			return fmt.Sprintf("'%v' is the designated receiver", name)
		}

		return "no designated receiver"
	}
}

// HelpCommand generates a Command runner to print
// the LogicLab commands and their help strings
func HelpCommand() Command {
	return func(env *Environment) string {
		return CommandHelp
	}
}

// ExitCommand generates a Command runner to
// abort the environment and close the REPL.
func ExitCommand() Command {
	return func(env *Environment) string {
		env.abort.Store(true)

		return ""
	}
}

// InvalidCommandError generates a Command runner
// to handle invalid commands declarations
func InvalidCommandError(err string) Command {
	return func(env *Environment) string {
		if err == "" {
			return "Invalid Command"
		}

		return fmt.Sprintf("Invalid Command: %v", err)
	}
}

// InvalidCommandErrorf generates a Command runner
// to handle invalid commands declarations
func InvalidCommandErrorf(format string, a ...any) Command {
	return func(env *Environment) string {
		return fmt.Sprintf(format, a...)
	}
}
