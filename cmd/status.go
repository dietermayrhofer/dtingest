package cmd

import (
	"fmt"

	"github.com/dietermayrhofer/dtingest/pkg/analyzer"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	statusOK    = color.New(color.FgGreen, color.Bold)
	statusError = color.New(color.FgRed, color.Bold)
	statusLabel = color.New(color.FgWhite, color.Bold)
	statusMuted = color.New(color.FgHiBlack)
	statusHead  = color.New(color.FgCyan, color.Bold)
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show connection status and system state",
	Long:  `Verifies connectivity to Dynatrace and displays the current system analysis.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		statusHead.Println("  Connection Status")
		statusMuted.Println("  " + "──────────────────────────────────────────")

		client, err := newDtClient()
		if err != nil {
			fmt.Printf("  %s  %s\n\n", statusLabel.Sprint("Dynatrace:"), statusError.Sprintf("✗ failed to connect: %v", err))
		} else {
			user, userErr := client.CurrentUser()
			if userErr != nil {
				fmt.Printf("  %s  %s\n", statusLabel.Sprint("Dynatrace:"), statusError.Sprintf("✗ error: %v", userErr))
			} else {
				fmt.Printf("  %s  %s\n", statusLabel.Sprint("Dynatrace:"), statusOK.Sprint("✓ connected"))
				userName := user.EmailAddress
				if userName == "" {
					userName = user.UserName
				}
				if userName != "" {
					fmt.Printf("  %s  %s\n", statusLabel.Sprint("Logged in as:"), color.New(color.FgWhite).Sprint(userName))
				}
			}
			fmt.Printf("  %s  %s\n\n", statusLabel.Sprint("Environment:"), color.New(color.FgWhite).Sprint(client.BaseURL()))
		}

		statusHead.Println("  System Analysis")
		statusMuted.Println("  " + "──────────────────────────────────────────")
		info, err := analyzer.AnalyzeSystem()
		if err != nil {
			fmt.Printf("  %s\n", statusError.Sprintf("✗ system analysis failed: %v", err))
			return nil
		}
		fmt.Println(info.Summary())
		return nil
	},
}
