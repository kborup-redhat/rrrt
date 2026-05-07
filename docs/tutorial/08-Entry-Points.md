---
title: "Chapter 8: Entry Points"
order: 8
---

# Chapter 8: Entry Points

## Introduction

RRRT has two binaries that work together: the CLI plugin (`oc-rrrt`) that runs on the user's workstation, and the analyzer (`rrrt-analyzer`) that runs inside a Kubernetes Job on the cluster. Think of them as a remote control and a robot — the CLI tells the cluster what to do, and the analyzer does the actual work close to the data.

## The CLI Entry Point (`cmd/oc-rrrt/main.go`)

### How It Becomes an oc Plugin

The binary is named `oc-rrrt`. Kubernetes and OpenShift discover CLI plugins by looking for executables on `$PATH` that follow the naming convention `oc-<name>` (or `kubectl-<name>`). When you place `oc-rrrt` on your PATH, running `oc rrrt report` automatically invokes it.

### Command Structure

```go
func main() {
    rootCmd := &cobra.Command{
        Use:   "oc-rrrt",
        Short: "Resource Rightsizing Reporting Tool",
    }

    reportCmd := &cobra.Command{
        Use:   "report",
        Short: "Generate a rightsizing report",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Load kubeconfig, create clientset, call cli.Run()
        },
    }

    reportCmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", nil,
        "Namespaces to analyze (can be repeated)")
    reportCmd.Flags().StringVarP(&output, "output", "o", "", "Output PDF path")
    reportCmd.Flags().IntVar(&lookbackDays, "lookback-days", 14,
        "Metrics lookback window in days")
    reportCmd.Flags().StringVar(&image, "image", "",
        "Override analyzer container image")
    reportCmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false,
        "Don't delete temporary namespace after completion")
    reportCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute,
        "Maximum time to wait for the Job")
}
```

The CLI uses cobra for command parsing and client-go's `clientcmd` for kubeconfig loading:

```go
loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
configOverrides := &clientcmd.ConfigOverrides{}
kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
    loadingRules, configOverrides)
config, err := kubeConfig.ClientConfig()
```

This respects the standard `KUBECONFIG` environment variable and `~/.kube/config` file, so it works exactly like `oc` itself.

### Version Injection

```go
var version = "dev"
```

The `version` variable is set at build time using linker flags:

```bash
go build -ldflags="-X main.version=${VERSION}" -o oc-rrrt ./cmd/oc-rrrt/
```

Running `oc rrrt version` prints the injected version. During development, it shows `dev`.

### Usage Examples

```bash
# Analyze all namespaces with defaults
oc rrrt report

# Analyze specific namespaces
oc rrrt report -n production -n staging

# Custom lookback and output path
oc rrrt report --lookback-days 30 -o monthly-report.pdf

# Use a specific analyzer image
oc rrrt report --image quay.io/kborup/rrrt:v0.1.0

# Keep the temporary namespace for debugging
oc rrrt report --keep-namespace
```

## The Analyzer Entry Point (`cmd/analyzer/main.go`)

The analyzer runs inside the Kubernetes Job. It reads its configuration from environment variables, does all the heavy lifting, and writes the PDF to `/output/report.pdf`.

### Configuration from Environment

```go
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
    // RRRT_CONSOLE_URL, RRRT_INCLUDE_OPENSHIFT, RRRT_PROMETHEUS_URL...
    return cfg
}
```

Environment variables are the standard way to configure containers. Each `RRRT_*` variable maps to a CLI flag that the user set when running `oc rrrt report` — the CLI passes these through to the Job spec (Chapter 7).

### Initialization Pipeline

```go
func main() {
    cfg := readConfig()

    // Build a Kubernetes scheme with core + apps types
    scheme := runtime.NewScheme()
    _ = corev1.AddToScheme(scheme)
    _ = appsv1.AddToScheme(scheme)

    // Create clients
    restConfig := ctrl.GetConfigOrDie()
    k8sClient, _ := client.New(restConfig, client.Options{Scheme: scheme})
    promClient := collector.NewPrometheusClient(cfg.PrometheusURL, "")
    ownerResolver := owner.NewResolver(k8sClient)

    // Create collector and run
    coll := collector.New(k8sClient, promClient, ownerResolver, ...)
    data, _ := coll.Collect(ctx, cfg.Namespaces)

    // Enrich report metadata
    data.ClusterName = detectClusterName(ctx, k8sClient)
    data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

    // Generate PDF
    pdf.Generate(data, "/output/report.pdf")
}
```

The analyzer uses `controller-runtime`'s `ctrl.GetConfigOrDie()` for in-cluster config (service account token from the mounted secret). It registers only the schemes it needs — `corev1` for Namespaces and `appsv1` for Deployments/StatefulSets. KubeVirt types are handled via the unstructured client so no scheme registration is needed.

### Cluster Name Detection

```go
func detectClusterName(ctx context.Context, c client.Client) string {
    cv := &unstructured.Unstructured{}
    cv.SetGroupVersionKind(schema.GroupVersionKind{
        Group: "config.openshift.io", Version: "v1", Kind: "ClusterVersion",
    })
    if err := c.Get(ctx, client.ObjectKey{Name: "version"}, cv); err != nil {
        return "Unknown Cluster"
    }
    id, found, _ := unstructured.NestedString(cv.Object, "spec", "clusterID")
    if found && id != "" {
        return id
    }
    return "OpenShift Cluster"
}
```

The cluster ID is read from the OpenShift `ClusterVersion` resource using the unstructured client. This avoids importing the OpenShift API types just for this one lookup.

## The Container Image

The `Containerfile` builds the analyzer into a minimal container:

```dockerfile
FROM golang:1.26 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o /tmp/rrrt-analyzer ./cmd/analyzer/

FROM registry.redhat.io/ubi9/ubi-minimal:latest
COPY --from=builder /tmp/rrrt-analyzer /usr/local/bin/rrrt-analyzer
RUN mkdir -p /output
USER 1001
ENTRYPOINT ["rrrt-analyzer"]
```

Key design choices:
- **Multi-stage build** keeps the final image small — only the static binary, no Go toolchain
- **`CGO_ENABLED=0`** produces a fully static binary that runs on any Linux base
- **UBI 9 Minimal** is Red Hat's hardened minimal base image, suitable for production use
- **`USER 1001`** runs as a non-root user for security
- **`/output`** directory is created for the PDF output

## Relationships

- The **CLI entry point** creates a Kubernetes client and calls the **CLI Orchestrator** (`cli.Run`)
- The **Analyzer entry point** creates the **Prometheus Client**, **Owner Resolver**, **Collector**, and **PDF Generator**
- Configuration flows from CLI flags → environment variables → `readConfig()` → component constructors

## Key Takeaways

- The CLI plugin follows the `oc-<name>` naming convention for automatic plugin discovery
- Kubeconfig loading uses the standard `clientcmd` library, respecting `KUBECONFIG` and `~/.kube/config`
- The analyzer reads all configuration from `RRRT_*` environment variables set by the Job spec
- Version is injected at build time via `-ldflags "-X main.version=..."`
- The container image uses a multi-stage build with UBI 9 Minimal and runs as non-root

This concludes our tour of the RRRT codebase. The architecture follows a clean separation: a thin CLI dispatches work to an on-cluster Job, which coordinates data collection, analysis, and report generation — all with automatic cleanup and security-first defaults.
