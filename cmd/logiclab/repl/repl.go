package repl

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/chzyer/readline"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

const PROMPT = "\u001b[32m>>> \u001b[0m"

type Repl struct {
	lab *core.Lab
	env *core.Environment

	abort  atomic.Bool
	active atomic.Bool

	memory map[string]any
}

func NewRepl(lab *core.Lab, envID string) (*Repl, error) {
	env, exists, err := lab.GetEnvironment(envID)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, core.ErrEnvironmentNotFound
	}

	return &Repl{
		lab: lab, env: env,
		memory: make(map[string]any),
	}, nil
}

func (repl *Repl) Abort()        { repl.abort.Store(true) }
func (repl *Repl) Aborted() bool { return repl.abort.Load() }
func (repl *Repl) Active() bool  { return repl.active.Load() }
func (repl *Repl) Activate()     { repl.active.Store(true) }
func (repl *Repl) Deactivate()   { repl.active.Store(false) }

// Start begins the Read-Evaluate-Print Loop for LogicLab Commands.
func (repl *Repl) Start() (err error) {
	// Setup readline instance
	term, _ := readline.NewEx(&readline.Config{
		Prompt:          PROMPT,
		InterruptPrompt: "^C",
	})

	defer func() { err = term.Close() }()

	for {
		// READ
		line, err := term.Readline()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) {
				return repl.Close(core.CLOSER)
			}

			fmt.Println("failed to readline:", err)
		}

		// skip empty line eval
		if line == "" {
			continue
		}

		// EVALUATE
		command := Parse(line)
		result := command(repl)

		// handle aborted environment
		if repl.Aborted() {
			return repl.Close(core.CLOSER)
		}

		// PRINT
		println(result)
	}
}

func (repl *Repl) FormatValue(value any) string {
	switch data := value.(type) {
	case []byte:
		format := "%v"
		if repl.env.Config.HexBytes {
			format = "%#x"
		}

		return fmt.Sprintf(format, data)

	case [32]byte:
		format := "%v"
		if repl.env.Config.HexBytes {
			format = "%#x"
		}

		return fmt.Sprintf(format, data)

	case *big.Int:
		if repl.env.Config.HexBigInt {
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

func (repl *Repl) GetDesignatedSender() (identifiers.Address, error) {
	if repl.env.Sender == "" {
		return identifiers.NilAddress, core.ErrSenderNotConf
	}

	return repl.env.Users[repl.env.Sender], nil
}

func (repl *Repl) Close(message string) error {
	if repl.Active() {
		println(message)
	}

	return repl.lab.Close()
}
