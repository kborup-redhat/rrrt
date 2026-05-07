package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "oc-rrrt",
		Short: "Resource Rightsizing Reporting Tool",
	}

	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a rightsizing report",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("report command not yet implemented")
			return nil
		},
	}

	reportCmd.Flags().StringSliceP("namespace", "n", nil, "Namespaces to analyze (can be repeated)")
	reportCmd.Flags().Bool("all-namespaces", true, "Analyze all accessible namespaces")
	reportCmd.Flags().StringP("output", "o", "", "Output PDF path")
	reportCmd.Flags().Int("lookback-days", 14, "Metrics lookback window in days")
	reportCmd.Flags().String("image", "", "Override analyzer container image")
	reportCmd.Flags().String("console-url", "", "OpenShift Console base URL")
	reportCmd.Flags().Bool("include-openshift", false, "Include openshift-* and kube-* namespaces")
	reportCmd.Flags().Bool("keep-namespace", false, "Don't delete temporary namespace after completion")
	reportCmd.Flags().Duration("timeout", 30*60*1e9, "Maximum time to wait for the Job")

	rootCmd.AddCommand(reportCmd)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
