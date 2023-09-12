package main

import (
	"fmt"
	"os"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/cmd/logiclab/cmds"
	"github.com/sarvalabs/go-moi/common/config"
)

type CliCommand struct {
	cmd *cobra.Command
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
		initCommand,
		startCommand,
		runCommand,
	)
}

func parseflags(cmd *cobra.Command) {
	// Set Persistent Flags
	cmd.PersistentFlags().StringP("env", "e", "./.logiclab", "directory path for the logiclab environment")
	cmd.PersistentFlags().BoolP("suppress", "s", false, "suppressed output")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (cli *CliCommand) Execute() {
	if err := cli.cmd.Execute(); err != nil {
		os.Exit(1)
	}
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
			fmt.Print(FIGLET)
		}
	},
}

// initCommand represents the 'logiclab init' command
var initCommand = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new LogicLab Environment.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab init' will create a new LogicLab Environment at the specified directory.
The directory can be specified with the '--env' flag. The directory must not already
exist and will be created and initialized with a new inventory.json file within it
`,
	Run: func(command *cobra.Command, args []string) {
		// Get the environment dirpath from the input flags (defaults if not provided)
		dirpath, _ := command.Flags().GetString("env")

		// Initialize a new lab environment
		if err := cmds.InitEnv(dirpath); err != nil {
			fmt.Println(err)

			return
		}

		fmt.Printf("successfully initialized LogicLab directory at '%v'\n", dirpath)
	},
}

// startCommand represents the 'logiclab start' command
var startCommand = &cobra.Command{
	Use:   "start",
	Short: "Start a LogicLab REPL in the specified environment.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab start' will start a new LogicLab REPL in the environment specified by the 
'--env' flag. The directory must already exist and contain an inventory.json file before 
being used to start a REPL. New environment can be initialized with 'logiclab init'
`,
	Run: func(command *cobra.Command, args []string) {
		// Get the environment dirpath from the input flags (defaults if not provided)
		dirpath, _ := command.Flags().GetString("env")

		// Print logiclab launch text if not suppressed
		if suppressed, _ := command.Flags().GetBool("suppress"); !suppressed {
			fmt.Println(fmt.Sprintf(LAUNCH, dirpath, DOCS))
		} else {
			fmt.Println(DIVIDE) // print just the divider if suppressed
		}

		// Load the command environment at the given dirpath
		env, err := cmds.LoadEnv(dirpath)
		if err != nil {
			fmt.Println(err)

			return
		}

		// Setup readline instance
		term, _ := readline.NewEx(&readline.Config{
			Prompt:          PROMPT,
			InterruptPrompt: "^C",
		})
		defer func() {
			if err = term.Close(); err != nil {
				fmt.Printf("failed to close readline: %v\n", err)
			}
		}()

		// Activate the REPL
		env.Activate()
		defer env.Deactivate()

		// Start the REPL
		_ = REPL(env, term)
	},
}

// runCommand represents the 'logiclab run' command
var runCommand = &cobra.Command{
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
		env, err := cmds.LoadEnv(dirpath)
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
}
