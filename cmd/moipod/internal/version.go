package internal

import (
	"fmt"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/spf13/cobra"
)

func GetVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the moipod version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("v%s\n", config.ProtocolVersion)
		},
	}
}
