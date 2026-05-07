package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kborup-redhat/rrrt/internal/collector"
	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/pdf"
	"github.com/kborup-redhat/rrrt/internal/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var version = "dev"

func main() {
	fmt.Println("rrrt-analyzer", version)

	cfg := readConfig()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	restConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating kubernetes client: %v\n", err)
		os.Exit(1)
	}

	promClient := collector.NewPrometheusClient(cfg.PrometheusURL, "")
	ownerResolver := owner.NewResolver(k8sClient)

	coll := collector.New(k8sClient, promClient, ownerResolver, cfg.ConsoleURL,
		cfg.LookbackDays, types.DefaultHeadroom, cfg.IncludeOpenShift)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	data, err := coll.Collect(ctx, cfg.Namespaces)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error collecting data: %v\n", err)
		os.Exit(1)
	}

	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.CLIVersion = version
	data.ImageVersion = version

	data.ClusterID = detectClusterID(ctx, k8sClient)
	data.ClusterName = detectFriendlyClusterName(ctx, k8sClient, cfg.ConsoleURL, data.ClusterID)

	if len(cfg.Namespaces) > 0 {
		data.Scope = "Namespaces: " + strings.Join(cfg.Namespaces, ", ")
	} else {
		data.Scope = "All namespaces"
	}

	vmCandidates := countCandidates(data.VMAnalyses)
	contCandidates := countCandidates(data.ContainerAnalyses)
	total := len(data.VMAnalyses) + len(data.ContainerAnalyses)
	fmt.Printf("Found %d rightsizing candidates out of %d resources. Generating PDF...\n",
		vmCandidates+contCandidates, total)

	if err := os.MkdirAll("/output", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	if err := pdf.Generate(data, "/output/report.pdf"); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating PDF: %v\n", err)
		os.Exit(1)
	}
	_ = os.WriteFile("/output/cluster-name.txt", []byte(data.ClusterName), 0644)

	fmt.Println("Report generated successfully at /output/report.pdf")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}

func readConfig() types.AnalyzerConfig {
	cfg := types.AnalyzerConfig{
		PrometheusURL: types.DefaultPrometheusURL,
		LookbackDays:  types.DefaultLookbackDays,
	}

	if ns := os.Getenv("RRRT_NAMESPACES"); ns != "" {
		cfg.Namespaces = strings.Split(ns, ",")
	}
	if days := os.Getenv("RRRT_LOOKBACK_DAYS"); days != "" {
		if d, err := strconv.Atoi(days); err == nil {
			cfg.LookbackDays = d
		}
	}
	if url := os.Getenv("RRRT_CONSOLE_URL"); url != "" {
		cfg.ConsoleURL = url
	}
	if inc := os.Getenv("RRRT_INCLUDE_OPENSHIFT"); inc == "true" {
		cfg.IncludeOpenShift = true
	}
	if url := os.Getenv("RRRT_PROMETHEUS_URL"); url != "" {
		cfg.PrometheusURL = url
	}

	return cfg
}

func detectClusterID(ctx context.Context, c client.Client) string {
	cv := &unstructured.Unstructured{}
	cv.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "ClusterVersion",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: "version"}, cv); err != nil {
		return ""
	}
	id, found, _ := unstructured.NestedString(cv.Object, "spec", "clusterID")
	if found {
		return id
	}
	return ""
}

func detectFriendlyClusterName(ctx context.Context, c client.Client, consoleURL, clusterID string) string {
	infra := &unstructured.Unstructured{}
	infra.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "Infrastructure",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: "cluster"}, infra); err == nil {
		name, found, _ := unstructured.NestedString(infra.Object, "status", "infrastructureName")
		if found && name != "" {
			return name
		}
	}

	if consoleURL != "" {
		if parsed, err := url.Parse(consoleURL); err == nil {
			host := parsed.Hostname()
			if idx := strings.Index(host, ".apps."); idx >= 0 {
				remainder := host[idx+len(".apps."):]
				parts := strings.SplitN(remainder, ".", 2)
				if parts[0] != "" {
					return parts[0]
				}
			}
		}
	}

	if clusterID != "" {
		return clusterID
	}

	return "openshift-cluster"
}

func countCandidates(analyses []types.ResourceAnalysis) int {
	count := 0
	for _, a := range analyses {
		if a.Direction != "" {
			count++
		}
	}
	return count
}
