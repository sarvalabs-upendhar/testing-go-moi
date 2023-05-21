package logiclab

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/jug/logiclab"
)

// logiclabCmd represents the 'logiclab' command
var logiclabCmd = &cobra.Command{
	Use:   "logiclab",
	Short: "Start LogicLab Sessions and Manage LogicLab Environments",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

The LogicLab runs within an environment that is represented by a directory 
that must contain an inventory.json file within it. This directory must be
passed with '--env' flag and can be created using the 'logiclab init command'.

For help with the LogicLab Commands, start the LogicLab and use the 'help' command
`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
		fmt.Println("\n" + logiclab.CommandHelp)
	},
}

// logiclabInitCmd represents the 'logiclab init' command
var logiclabInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new LogicLab Environment",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab init' will create a new LogicLab Environment at the specified directory.
The directory can be specified with the '--env' flag. The directory must not already
exist and will be created and initialized with a new inventory.json file within it
`,
	Run: func(cmd *cobra.Command, args []string) {
		env, _ := cmd.Flags().GetString("env")
		if err := logiclab.InitEnvironment(env); err != nil {
			fmt.Println(err)

			return
		}

		fmt.Printf("successfully initialized LogicLab directory at '%v'\n", env)
	},
}

// logiclabStartCmd represents the 'logiclab start' command
var logiclabStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a LogicLab REPL in the specified Environment",
	Long: `The LogicLab is a sandbox environment for simulating logic 
calls and participant interactions with logics on MOI.

'logiclab start' will start a new LogicLab REPL in the environment specified by the 
'--env' flag. The directory must already exist and contain an inventory.json file before 
being used to start a REPL. New environment can be initialized with 'logiclab init'
`,
	Run: func(cmd *cobra.Command, args []string) {
		env, _ := cmd.Flags().GetString("env")

		logiclab, err := logiclab.LoadEnvironment(env)
		if err != nil {
			fmt.Println(err)

			return
		}

		logiclab.StartREPL(os.Stdin, os.Stdout)
	},
}

func GetCommand() *cobra.Command {
	parseFlags(logiclabCmd)
	setupSubCommands(logiclabCmd)

	return logiclabCmd
}

func parseFlags(cmd *cobra.Command) {
	// Persistent Flags
	cmd.PersistentFlags().StringP(
		"env", "e", "./.artifacts/logiclab", "directory path for the logiclab environment",
	)
}

func setupSubCommands(cmd *cobra.Command) {
	cmd.AddCommand(logiclabInitCmd)
	cmd.AddCommand(logiclabStartCmd)
}
