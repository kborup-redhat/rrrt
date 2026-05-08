package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kborup-redhat/rrrt/internal/cli"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "oc-rrrt",
		Short: "Resource Rightsizing Reporting Tool",
	}

	var namespaces []string
	var output string
	var lookbackDays int
	var image string
	var consoleURL string
	var includeOpenShift bool
	var keepNamespace bool
	var timeout time.Duration
	var noOVRO bool
	var ovroEndpoint string

	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a rightsizing report",
		RunE: func(cmd *cobra.Command, args []string) error {
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

			config, err := kubeConfig.ClientConfig()
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				return fmt.Errorf("creating clientset: %w", err)
			}

			return cli.Run(context.Background(), config, clientset, cli.RunConfig{
				Namespaces:    namespaces,
				Output:        output,
				LookbackDays:  lookbackDays,
				Image:         image,
				ConsoleURL:    consoleURL,
				IncludeOS:     includeOpenShift,
				KeepNamespace: keepNamespace,
				Timeout:       timeout,
				Version:       version,
				NoOVRO:        noOVRO,
				OVROEndpoint:  ovroEndpoint,
			})
		},
	}

	reportCmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", nil, "Namespaces to analyze (can be repeated)")
	reportCmd.Flags().StringVarP(&output, "output", "o", "", "Output PDF path")
	reportCmd.Flags().IntVar(&lookbackDays, "lookback-days", 30, "Metrics lookback window in days")
	reportCmd.Flags().StringVar(&image, "image", "", "Override analyzer container image")
	reportCmd.Flags().StringVar(&consoleURL, "console-url", "", "OpenShift Console base URL")
	reportCmd.Flags().BoolVar(&includeOpenShift, "include-openshift", false, "Include openshift-* and kube-* namespaces")
	reportCmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false, "Don't delete temporary namespace after completion")
	reportCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Maximum time to wait for the Job")
	reportCmd.Flags().BoolVar(&noOVRO, "no-ovro", false, "Force using Thanos even when OVRO is detected")
	reportCmd.Flags().StringVar(&ovroEndpoint, "ovro-endpoint", "", "Override VictoriaMetrics endpoint URL (implies OVRO mode)")

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
