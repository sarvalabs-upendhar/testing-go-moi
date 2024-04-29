package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/cmd/logiclab/api"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/cmd/logiclab/repl"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
)

func init() {
	engineio.RegisterEngine(pisa.NewEngine())
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	CliRoot().Execute()
}

type CliCommand struct {
	cmd *cobra.Command
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (cli *CliCommand) Execute() {
	if err := cli.cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func CliRoot() *CliCommand {
	root := &CliCommand{
		cmd: &cobra.Command{
			Use:   "logiclab",
			Short: "Start LogicLab sessions and manage LogicLab environments.",
			Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

The LogicLab runs within an environment that is represented by a directory 
that must contain an inventory.json file within it. This directory must be
passed with '--env' flag and can be created using the 'logiclab init command'.

For help with the LogicLab Commands, start the LogicLab and use the 'help' command
`,
			Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
		},
	}

	parseflags(root.cmd)
	root.RegisterSubCommands()

	return root
}

func (cli *CliCommand) RegisterSubCommands() {
	cli.cmd.AddCommand(
		versionCommand,
		startCommand,
		// runCommand,
	)
}

func parseflags(cmd *cobra.Command) {
	// -r | --root [string]
	cmd.PersistentFlags().StringP(
		"root", "r", core.DefaultRootPath,
		fmt.Sprintf("root directory path for logiclab. defaults to '%v'", core.DefaultRootPath),
	)

	// -s | --suppress [bool]
	cmd.PersistentFlags().BoolP(
		"suppress", "s", false,
		"turn on suppression. OFF by default",
	)

	// -m | --mode [string]
	cmd.PersistentFlags().StringP(
		"mode", "m", "API",
		"mode for running logiclab. valid values are 'API' (default) and 'REPL'",
	)

	// -e | --env [string]
	cmd.PersistentFlags().StringP(
		"env", "e", core.DefaultEnvironment,
		fmt.Sprintf("logiclab environment to activate. only applicable in REPL mode. defaults to '%v'", core.DefaultEnvironment), //nolint:lll
	)
}

// versionCommand represents the 'logiclab version' command
var versionCommand = &cobra.Command{
	Use:   "version",
	Short: "Print the LogicLab version",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab version' will print the version of the LogicLab installation
`,
	Run: func(command *cobra.Command, args []string) {
		// Print logiclab version
		fmt.Printf("LogicLab %v\n", config.ProtocolVersion)
		// Print logiclab figlet if not suppressed
		if suppressed, _ := command.Flags().GetBool("suppress"); !suppressed {
			fmt.Print(core.FIGLET)
		}
	},
}

// startCommand represents the 'logiclab start' command
var startCommand = &cobra.Command{
	Use:   "start",
	Short: "Start LogicLab in the specified mode.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab start' will start a new LogicLab API/REPL in the environment specified by the 
'--env' flag. The directory must already exist and contain an inventory.json file before 
being used to start it. New environment can be initialized with 'logiclab init'
`,
	Run: func(command *cobra.Command, args []string) {
		// Get the lab root dirpath from the input flags (defaults if not provided)
		root, _ := command.Flags().GetString("root")
		root, _ = filepath.Abs(root)

		// Create a logiclab instance
		lab, err := core.NewLab(root)
		if err != nil {
			fmt.Println(err)
			return //nolint:nlreturn
		}

		// Start the interrupt handler
		handler := lab.HandleInterrupt()
		go handler()

		// Get the mode for starting the logiclab environment (REPL/API)
		mode, _ := command.Flags().GetString("mode")

		// Print logiclab launch text if not suppressed
		if suppressed, _ := command.Flags().GetBool("suppress"); !suppressed {
			fmt.Println(fmt.Sprintf(core.LAUNCH, root, core.DOCS, mode))
		} else {
			fmt.Println(core.DIVIDE) // print just the divider if suppressed
		}

		switch strings.ToUpper(mode) {
		case "API":
			// Create a new API instance and start it
			api := api.NewAPI(lab)
			_ = api.Start()

		case "REPL":
			// Get the environment to use in the REPL
			env, _ := command.Flags().GetString("env")
			// Load up the REPL instance
			repl, err := repl.NewRepl(lab, env)
			if err != nil {
				fmt.Println(err)
				return //nolint:nlreturn
			}

			// Activate the REPL
			repl.Activate()
			defer repl.Deactivate()

			// Start the REPL
			_ = repl.Start()

		default:
			fmt.Println("invalid mode")
			return //nolint:nlreturn
		}
	},
}

// runCommand represents the 'logiclab run' command
/*var runCommand = &cobra.Command{
	Use:   "run [formula]",
	Short: "Run a LogicLab formula.",
	Long: `The LogicLab is a sandbox environment for simulating logic
calls and participant interactions with logics on MOI.

'logiclab run' will run a LogicLab formula in the specified environment.
The formula file path should be provided as an argument. The environment
directory must already exist and contain an inventory.json file.
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("accepts 1 or 2 arguments, received %d", len(args))
		}

		return nil
	},

	Run: func(command *cobra.Command, args []string) {
		// Get the environment dirpath from the input flags (defaults if not provided)
		dirpath, _ := command.Flags().GetString("env")
		suppress, _ := command.Flags().GetBool("suppress")

		// Load the command environment at the given dirpath
		env, err := core.LoadLab(dirpath)
		if err != nil {
			fmt.Println(err)

			return
		}

		// Get the formula path from the arguments
		formula := args[0]
		// Run the command formula in the command env
		err = cmds.RunFormula(env, formula, suppress)

		if err != nil {
			fmt.Println(err)

			return
		}
	},
}*/
