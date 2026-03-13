package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the dtingest version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dtingest %s\n", Version)
	},
}
