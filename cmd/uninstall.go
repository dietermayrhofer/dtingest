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

func init() {
	uninstallCmd.AddCommand(uninstallKubernetesCmd)
}
