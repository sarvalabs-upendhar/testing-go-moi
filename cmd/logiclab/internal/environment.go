package internal

import (
	"fmt"
	"io"
	"math/big"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
)

func init() {
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
}

const (
	replPrompt  = ">> "
	replDivider = "======================================================"
	replStrike  = "------------------------------------------------------"
	replFiglet  = "" + replDivider + "\n" +
		"dP                   oo          dP          dP       \n" +
		"88                               88          88       \n" +
		"88 .d8888b. .d8888b. dP .d8888b. 88 .d8888b. 88d888b. \n" +
		"88 88'  `88 88'  `88 88 88'  `\"\" 88 88'  `88 88'  `88 \n" +
		"88 88.  .88 88.  .88 88 88.  ... 88 88.  .88 88.  .88 \n" +
		"dP `88888P' `8888P88 dP `88888P' dP `88888P8 88Y8888' \n" +
		"                 .88                                  \n" +
		"             d8888P                                   \n" +
		replDivider + "\n"
)

// Environment represents the logic lab runtime environment
type Environment struct {
	// abort represents the abort flag
	abort atomic.Bool
	repl  atomic.Bool

	// input is the command read buffer
	input io.Reader
	// output is result write buffer
	output io.Writer

	// memory contains any runtime value assigned with 'set'
	memory map[string]any
	// inventory contains all the items the lab has
	// access to, such as participants and logics
	inventory *Inventory
	// directory is the path to the directory containing all saved lab items
	directory string
}

// LoadEnvironment loads an existing LogicLab environment.
// Fails it there isn't a directory at the given path with an inventory.json file in it.
func LoadEnvironment(dirpath string) (*Environment, error) {
	if !pathExists(dirpath) {
		return nil, errors.Errorf("could not start LogicLab at directory '%v': directory does not exist", dirpath)
	}

	inventory := new(Inventory)
	if err := inventory.load(dirpath); err != nil {
		return nil, errors.Wrap(err, "could not start LogicLab at directory '%v':")
	}

	return &Environment{
		directory: dirpath,
		inventory: inventory,
		memory:    make(map[string]any),
	}, nil
}

// InitEnvironment initializes a new LogicLab environment.
// Fails if there already exists a directory at the given path.
// The directory is created and initialized with a new inventory.json file.
func InitEnvironment(dirpath string) error {
	if pathExists(dirpath) {
		return errors.Errorf("could not initialize LogicLab directory at '%v': directory already exists", dirpath)
	}

	if err := createDir(dirpath); err != nil {
		return errors.Wrapf(err, "could not initialize LogicLab directory at '%v'", dirpath)
	}

	inventory := Inventory{
		labdir: dirpath,

		Config: LabConfig{
			BaseFuel: engineio.NewFuel(10000),
			HexBig:   true,
			HexBytes: true,
		},

		Participants:  make(map[string]common.Address),
		LogicAccounts: make(map[string]common.LogicID),
	}

	if err := inventory.save(); err != nil {
		return errors.Wrap(err, "could not initialize LogicLab directory: failed to create inventory file")
	}

	return nil
}

// StartREPL starts Read-Evaluate-Print Loop for LogicLab Commands.
// Input commands are read from the input buffer and output results are written to the output buffer.
func (env *Environment) StartREPL(in io.Reader, out io.Writer) {
	// Set REPL flag to ON
	env.repl.Store(true)
	defer env.repl.Store(false)

	// Start signal handler
	env.handleSignals()

	// Set up the IO buffers
	env.input, env.output = in, out

	rl, err := readline.New(">> ")
	if err != nil {
		env.write(fmt.Sprintf("Failed to initialize readline: %v", err))
		return //nolint:nlreturn
	}

	defer func() {
		err := rl.Close()
		if err != nil {
			env.write(fmt.Sprintf("Failed to initialize readline: %v", err))
		}
	}()

	// Launch Sequence
	env.write(replFiglet)
	env.write("LogicLab Initialized @ " + env.directory)
	env.write("LogicLab Documentation: https://moichain-docs.pages.dev/docs/logiclab-cli")

	env.write("Starting LogicLab REPL ... (use 'exit' or ctrl-c to close the REPL)")
	env.write(replDivider)

REPL:
	// Start Read-Evaluate-Print Loop
	for {
		// Write line prompt
		_, _ = fmt.Fprint(env.output, replPrompt)
		// Scan user input
		line, err := rl.Readline()
		if err != nil {
			fmt.Println("Failed to initialize readline", err)
		}

		// Continue for empty line
		if line == "" {
			continue
		}

		// Collect the scanned text and parse into a command
		command := ParseCommand(line)
		// Perform the command
		result := command(env)
		// If abort is detected, close the lab and break from REPL
		if env.abort.Load() {
			_ = env.close()

			break REPL
		}

		// Write the output of the command run
		env.write(result)
	}
}

func (env *Environment) RunScript(scriptPath string, suppress bool) error {
	file, err := os.Open(scriptPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := io.Reader(file)
	buffer := make([]byte, 1024*1024) // Buffer to read the file content

	var lastOutput string // Store the output of the last executed command

	for {
		n, err := reader.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}

			return err
		}

		content := string(buffer[:n])
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line) // Trim leading and trailing whitespace

			// Skip empty lines
			if line == "" {
				continue
			} else if !suppress {
				fmt.Println(">> ", line)
			}

			// Check if the abort flag is set
			if env == nil {
				return errors.New("environment is nil")
			}

			command := ParseCommand(line)
			result := command(env)

			if env.abort.Load() {
				_ = env.close()

				break
			}

			if !suppress {
				fmt.Println(result)
			}

			lastOutput = result
		}
	}

	// Print the last executed command's output
	if suppress && lastOutput != "" {
		fmt.Println(lastOutput)
	}

	return nil
}

// GetReference implements the engineio.ReferenceVal
func (env *Environment) GetReference(ref engineio.ReferenceVal) (any, bool) {
	val, ok := env.memory[string(ref)]

	return val, ok
}

// write outputs the given content to the environment output buffer.
// The given output is always suffixed with a new line character.
func (env *Environment) write(s string) {
	_, _ = io.WriteString(env.output, fmt.Sprintf("%v\n", s))
}

// handleSignals sets ups the system interrupt handler.
// Any system level interrupt will abort all execution exit gracefully.
func (env *Environment) handleSignals() {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signals
		switch sig {
		case os.Interrupt, syscall.SIGTERM, syscall.SIGKILL:
			// Abort any running execution
			env.abort.Store(true)
			// Close the environment
			_ = env.close()

			// Exit with code 0
			os.Exit(0)
		}
	}()
}

// close exits the environment by saving the lab session to the inventory
func (env *Environment) close() error {
	if env.repl.Load() {
		env.write("\r" + replDivider + "\nClosing LogicLab REPL")
	}

	// Flush the inventory file to the directory
	if err := env.inventory.save(); err != nil {
		return err
	}

	return nil
}

// SetValueCommand generates a Command runner to set the value
// of an identifier to a given value in the environment memory
func SetValueCommand(ident string, value any) Command {
	return func(env *Environment) string {
		env.memory[ident] = value

		return fmt.Sprintf("'%v' has been set to %v", ident, env.formatValue(value))
	}
}

// GetValueCommand generates a Command runner to get the
// value of an identifier from the environment memory
func GetValueCommand(ident string) Command {
	return func(env *Environment) string {
		value, ok := env.memory[ident]
		if !ok {
			return fmt.Sprintf("no value set for '%v'", ident)
		}

		return fmt.Sprintf("'%v' is set to %v", ident, env.formatValue(value))
	}
}

func SetConfigCommand(param string, value any) Command {
	return func(env *Environment) string {
		switch param {
		case "basefuel":
			test := env.formatValue(value)
			n := new(big.Int)
			n, err := n.SetString(test, 10)

			if !err {
				return fmt.Sprintf("error converting '%v' has not been set to %v", param, env.formatValue(value))
			}

			env.inventory.Config.BaseFuel.Set(n)

			return fmt.Sprintf("'%v' has been set to %v", param, env.formatValue(value))

		case "hexbig":
			test := env.formatValue(value)

			b, err := strconv.ParseBool(test)
			if err != nil {
				return fmt.Sprintf("error converting '%v' has not been set to %v", param, env.formatValue(value))
			}

			env.inventory.Config.HexBig = b

			return fmt.Sprintf("'%v' has been set to %v", param, env.formatValue(value))

		case "hexbytes":
			test := env.formatValue(value)

			b, err := strconv.ParseBool(test)
			if err != nil {
				return fmt.Sprintf("error converting '%v' has not been set to %v", param, env.formatValue(value))
			}

			env.inventory.Config.HexBytes = b

			return fmt.Sprintf("'%v' has been set to %v", param, env.formatValue(value))

		default:
			return fmt.Sprintf("unsupported config param: %v", param)
		}
	}
}

func GetConfigCommand(param string) Command {
	return func(env *Environment) string {
		switch param {
		case "basefuel":
			return env.inventory.Config.BaseFuel.String()
		case "hexbig":
			return env.formatValue(env.inventory.Config.HexBig)
		case "hexbytes":
			return env.formatValue(env.inventory.Config.HexBytes)
		default:
			return fmt.Sprintf("unsupported config param: %v", param)
		}
	}
}

func (env *Environment) formatValue(value any) string {
	switch data := value.(type) {
	case []byte:
		format := "%v"
		if env.inventory.Config.HexBytes {
			format = "%#x"
		}

		return fmt.Sprintf(format, data)

	case [32]byte:
		format := "%v"
		if env.inventory.Config.HexBytes {
			format = "%#x"
		}

		return fmt.Sprintf(format, data)

	case *big.Int:
		if env.inventory.Config.HexBig {
			return fmt.Sprintf("big(%#x)", data.Bytes())
		} else {
			// Obtain absolute value
			abs := new(big.Int).Abs(data)

			// Format into base10 string
			result := abs.String()
			// Prepend negative sign if negative
			if data.Sign() == -1 {
				result = string('-') + result
			}

			return fmt.Sprintf("big(%v)", result)
		}

	default:
		return fmt.Sprintf("%v", data)
	}
}

func (env *Environment) Driver() engineio.EnvDriver {
	return engineio.NewEnvObject(time.Now().Unix(), big.NewInt(1))
}
