package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/cmd/logiclab/internal"
)

// LogicLabCommandDocWebsite represents the link to the LogicLab Web Documentation
const LogicLabCommandDocWebsite = "https://docs.moi.technology/docs/logic-lab"

func main() {
	NewRootCommand().Execute()
}

type Command struct {
	baseCmd *cobra.Command
}

// logiclabDocsCmd represents the 'logiclab docs' command
var logiclabDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the LogicLab Command Documentation",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab docs' will print the command documentation for LogicLab.
It can also be viewed at ` + LogicLabCommandDocWebsite + ` 
or viewed within a LogicLab Session using the 'help' command.
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("\n" + internal.CommandHelp)
	},
}

// logiclabInitCmd represents the 'logiclab init' command
var logiclabInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new LogicLab Environment.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab init' will create a new LogicLab Environment at the specified directory.
The directory can be specified with the '--env' flag. The directory must not already
exist and will be created and initialized with a new inventory.json file within it
`,
	Run: func(cmd *cobra.Command, args []string) {
		env, _ := cmd.Flags().GetString("env")
		if err := internal.InitEnvironment(env); err != nil {
			fmt.Println(err)

			return
		}

		fmt.Printf("successfully initialized LogicLab directory at '%v'\n", env)
	},
}

// logiclabStartCmd represents the 'logiclab start' command
var logiclabStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a LogicLab REPL in the specified environment.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab start' will start a new LogicLab REPL in the environment specified by the 
'--env' flag. The directory must already exist and contain an inventory.json file before 
being used to start a REPL. New environment can be initialized with 'logiclab init'
`,
	Run: func(cmd *cobra.Command, args []string) {
		env, _ := cmd.Flags().GetString("env")

		logiclab, err := internal.LoadEnvironment(env)
		if err != nil {
			fmt.Println(err)

			return
		}

		logiclab.StartREPL(os.Stdin, os.Stdout)
	},
}

var logiclabRunCmd = &cobra.Command{
	Use:   "run [labscript]",
	Short: "Run a LogicLab script.",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab run' will run a LogicLab script in the specified environment.
The script file path should be provided as an argument. The environment
directory must already exist and contain an inventory.json file.
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("accepts 1 or 2 arguments, received %d", len(args))
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		env, _ := cmd.Flags().GetString("env")
		scriptPath := args[0]                          // Get the script path from the arguments
		suppress, _ := cmd.Flags().GetBool("suppress") // Get the value of --suppress flag

		logiclab, err := internal.LoadEnvironment(env)
		if err != nil {
			fmt.Println(err)

			return
		}

		if suppress {
			err = logiclab.RunScript(scriptPath, true) // Pass true to RunScript if suppress flag is present
		} else {
			err = logiclab.RunScript(scriptPath, false) // Pass false to RunScript if suppress flag is not present
		}

		if err != nil {
			fmt.Println(err)

			return
		}
	},
}

func init() {
	logiclabRunCmd.Flags().Bool("suppress", false, "suppress outputs during script execution")
}

func parseFlags(cmd *cobra.Command) {
	// Persistent Flags
	cmd.PersistentFlags().StringP(
		"env", "e", "./.artifacts/logiclab", "directory path for the logiclab environment",
	)
}

func NewRootCommand() *Command {
	rc := &Command{
		baseCmd: &cobra.Command{
			Use:   "logiclab",
			Short: "Start LogicLab sessions and manage LogicLab environments.",
			Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

The LogicLab runs within an environment that is represented by a directory 
that must contain an inventory.json file within it. This directory must be
passed with '--env' flag and can be created using the 'logiclab init command'.

For help with the LogicLab Commands, start the LogicLab and use the 'help' command
`,
			Run: func(cmd *cobra.Command, args []string) {
				_ = cmd.Help()
			},
		},
	}

	parseFlags(rc.baseCmd)
	rc.RegisterSubCommands()

	return rc
}

func (rc *Command) RegisterSubCommands() {
	rc.baseCmd.AddCommand(
		logiclabInitCmd,
		logiclabStartCmd,
		logiclabRunCmd,
		logiclabDocsCmd,
	)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func (rc *Command) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
