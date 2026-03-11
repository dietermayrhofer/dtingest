package cmd

import (
	"github.com/dietermayrhofer/dtingest/pkg/installer"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <method>",
	Short: "Uninstall a Dynatrace ingestion method",
}

var uninstallKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Remove Dynatrace Operator and DynaKube resources from Kubernetes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installer.UninstallKubernetes()
	},
}

var uninstallOneAgentCmd = &cobra.Command{
	Use:   "oneagent",
	Short: "Uninstall Dynatrace OneAgent from this host",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		return installer.UninstallOneAgent(dryRun)
	},
}

var uninstallAWSCmd = &cobra.Command{
	Use:   "aws",
	Short: "Remove the Dynatrace AWS CloudFormation stack and monitoring configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		envURL, token, _ := getDtEnvironment()
		return installer.UninstallAWS(envURL, token, dryRun)
	},
}

var uninstallOtelCmd = &cobra.Command{
	Use:   "otel-collector",
	Short: "Kill running OTel Collector processes and remove installation files",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		return installer.UninstallOtelCollector(dryRun)
	},
}

func init() {
	uninstallOneAgentCmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	uninstallAWSCmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	uninstallOtelCmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	uninstallCmd.AddCommand(uninstallKubernetesCmd)
	uninstallCmd.AddCommand(uninstallOneAgentCmd)
	uninstallCmd.AddCommand(uninstallAWSCmd)
	uninstallCmd.AddCommand(uninstallOtelCmd)
}
