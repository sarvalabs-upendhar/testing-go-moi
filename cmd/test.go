package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// testnetCmd represents the testnet command
var testnetCmd = &cobra.Command{
	Use:   "test",
	Short: "testnet consists of all the util functions needed for deploying testnet",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("testnet called")
	},
}

func init() {
	rootCmd.AddCommand(testnetCmd)
}
