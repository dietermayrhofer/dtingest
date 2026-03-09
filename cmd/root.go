package cmd

import (
	"fmt"
	"os"

	"github.com/dietermayrhofer/dtingest/pkg/dtctl"
	"github.com/spf13/cobra"
)

var contextOverride string
var environmentFlag string
var accessTokenFlag string
var platformTokenFlag string

var rootCmd = &cobra.Command{
	Use:   "dtingest",
	Short: "Dynatrace Ingest CLI — analyze systems and deploy observability",
	Long: `dtingest analyzes your system and deploys the best Dynatrace ingestion method.

Authentication is handled by dtctl. Run 'dtctl auth login' or
'dtctl config set-context' to configure your Dynatrace environment,
then use dtingest commands to analyze and instrument your system.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return dtctl.EnsureInstalled()
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&contextOverride, "context", "", "override the dtctl context to use")
	rootCmd.PersistentFlags().StringVar(&environmentFlag, "environment", "", "Dynatrace environment URL (overrides dtctl context; also read from DT_ENVIRONMENT)")
	rootCmd.PersistentFlags().StringVar(&accessTokenFlag, "access-token", "", "Dynatrace API access token for methods that require it (also read from DT_ACCESS_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&platformTokenFlag, "platform-token", "", "Dynatrace platform token (dt0s16.*) for AWS installer (also read from DT_PLATFORM_TOKEN)")

	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(recommendCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(statusCmd)
}
