---
title: "Chapter 7: CLI Orchestrator"
order: 7
---

# Chapter 7: CLI Orchestrator

## Introduction

The CLI orchestrator is the conductor of the entire operation. When you run `oc rrrt report`, it manages a sequence of steps: creating a temporary namespace, setting up RBAC, dispatching the analyzer Job, streaming logs, downloading the report, and cleaning everything up. Think of it as a stage manager — it coordinates all the actors (RBAC, Job, cleanup) to execute the show from start to finish.

## How It Works

The orchestrator lives in `internal/cli/run.go` and is the only function the CLI entry point calls.

### The Run Function

```go
func Run(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset,
    cfg RunConfig) error {

    // Generate a unique namespace name
    randBytes := make([]byte, 4)
    rand.Read(randBytes)
    nsName := "rrrt-" + hex.EncodeToString(randBytes) // e.g., "rrrt-a1b2c3d4"

    // Auto-detect console URL if not provided
    if cfg.ConsoleURL == "" {
        url, _ := detectConsoleURL(ctx, clientset)
        cfg.ConsoleURL = url
    }

    // Create namespace
    createNamespace(ctx, clientset, nsName)

    // Register cleanup handler (namespace + RBAC)
    cleanup := NewCleanup(clientset, nsName, nil, cfg.KeepNamespace)
    cleanup.RegisterSignalHandler(cancel)

    // Create RBAC resources
    createServiceAccount(ctx, clientset, nsName)
    ensureClusterRole(ctx, clientset)
    crbNames, _ := createClusterRoleBindings(ctx, clientset, nsName)

    // Build and dispatch the Job
    job := buildJob(JobConfig{...})
    clientset.BatchV1().Jobs(nsName).Create(ctx, job, ...)

    // Wait for pod, stream logs, wait for completion
    podName, _ := waitForPod(ctx, clientset, nsName, cfg.Timeout)
    streamLogs(ctx, clientset, nsName, podName)
    waitForJobCompletion(ctx, clientset, nsName, cfg.Timeout)

    // Download the report
    copyFromPod(ctx, config, clientset, nsName, podName,
        "/output/report.pdf", outputPath)

    // Clean up
    cleanup.Run()
}
```

Every step has error handling that triggers cleanup before returning. This ensures the temporary namespace and RBAC resources don't leak even if something fails partway through.

### Console URL Detection (`console.go`)

```go
func detectConsoleURL(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
    cm, err := clientset.CoreV1().ConfigMaps("openshift-console").Get(
        ctx, "console-config", metav1.GetOptions{})
    // Parse YAML to extract clusterInfo.consoleBaseAddress
}
```

The console URL is read from the `console-config` ConfigMap in the `openshift-console` namespace. This is a standard OpenShift resource that contains the cluster's console base address. If detection fails, a warning is printed but the run continues — the report will just have empty console links.

### RBAC Setup (`rbac.go`)

The RBAC manager creates four resources:

1. **Namespace** — ephemeral, named `rrrt-<random>`
2. **ServiceAccount** — `rrrt-analyzer` in the ephemeral namespace
3. **ClusterRole** — `rrrt-analyzer` with read-only permissions for VMs, Deployments, StatefulSets, Namespaces, and ConfigMaps. Uses `ensureClusterRole` which creates-or-updates, so it's idempotent
4. **ClusterRoleBindings** — two bindings: one for the custom role, one for `cluster-monitoring-view` (required to query Prometheus/Thanos)

```go
Rules: []rbacv1.PolicyRule{
    {
        APIGroups: []string{"kubevirt.io"},
        Resources: []string{"virtualmachines", "virtualmachineinstances"},
        Verbs:     []string{"get", "list"},
    },
    {
        APIGroups: []string{"apps"},
        Resources: []string{"deployments", "statefulsets"},
        Verbs:     []string{"get", "list"},
    },
    // namespaces and configmaps...
}
```

All permissions are **read-only** — the analyzer never modifies cluster resources.

### Job Builder (`job.go`)

```go
func buildJob(cfg JobConfig) *batchv1.Job {
    deadlineSeconds := int64(cfg.Timeout.Seconds())
    backoffLimit := int32(0)

    env := []corev1.EnvVar{
        {Name: "RRRT_LOOKBACK_DAYS", Value: fmt.Sprintf("%d", cfg.LookbackDays)},
        {Name: "RRRT_CONSOLE_URL", Value: cfg.ConsoleURL},
        // ...
    }

    return &batchv1.Job{
        Spec: batchv1.JobSpec{
            ActiveDeadlineSeconds: &deadlineSeconds,
            BackoffLimit:          &backoffLimit, // no retries
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: "rrrt-analyzer",
                    RestartPolicy:      corev1.RestartPolicyNever,
                    // ...
                },
            },
        },
    }
}
```

Configuration is passed to the analyzer container via environment variables. `BackoffLimit: 0` prevents Kubernetes from retrying a failed Job — if the analysis fails, the user should investigate rather than silently retrying.

### Log Streaming

```go
func streamLogs(ctx context.Context, clientset *kubernetes.Clientset,
    namespace, podName string) error {
    req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
        Follow: true,
    })
    stream, err := req.Stream(ctx)
    defer func() { _ = stream.Close() }()

    scanner := bufio.NewScanner(stream)
    for scanner.Scan() {
        line := scanner.Text()
        var progress struct {
            Phase string `json:"phase"`
            // ...
        }
        if json.Unmarshal([]byte(line), &progress) == nil && progress.Phase != "" {
            fmt.Printf("[%d/%d] Analyzing %s %s/%s...\n",
                progress.Index, progress.Total, progress.Phase, ...)
        }
    }
}
```

The log streamer follows the Job's pod logs in real time. It parses the JSON progress messages emitted by the Collector (Chapter 4) and reformats them as human-friendly progress lines like `[3/15] Analyzing vms default/my-database...`.

### File Transfer (`copy.go`)

```go
func copyFromPod(ctx context.Context, config *rest.Config,
    clientset *kubernetes.Clientset, namespace, podName,
    srcPath, destPath string) error {

    // exec tar in the pod, stream stdout through a pipe
    exec.StreamWithContext(ctx, remotecommand.StreamOptions{
        Stdout: pw, Stderr: os.Stderr,
    })

    // read tar stream, validate path, write file
    tr := tar.NewReader(pr)
    header, _ := tr.Next()
    clean := filepath.Clean(header.Name)
    if strings.Contains(clean, "..") {
        return fmt.Errorf("tar entry contains path traversal: %s", header.Name)
    }
    // write to destPath...
}
```

The report is copied from the completed pod using `tar` via the exec API — the same mechanism as `oc cp`. Path traversal validation prevents a compromised pod from writing files outside the intended destination.

### Cleanup (`cleanup.go`)

```go
func (c *Cleanup) RegisterSignalHandler(cancel context.CancelFunc) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        cancel()
        c.Run()
        os.Exit(1)
    }()
}

func (c *Cleanup) Run() {
    if c.keepNamespace { return }
    // Delete ClusterRoleBindings, then namespace
}
```

Cleanup handles both normal completion and signal interrupts (Ctrl+C). The `--keep-namespace` flag allows users to skip cleanup for debugging. If cleanup fails, it prints the manual `oc delete` commands the user needs to run.

## Relationships

- Called by the **CLI Entry Point** (`cmd/oc-rrrt/main.go`) with cobra-parsed flags
- Uses **RBAC Manager**, **Job Builder**, **Console URL Detector**, and **Cleanup Handler**
- Streams and parses progress logs from the **Collector** running inside the Job
- Downloads the PDF from the **PDF Generator** via tar/exec

## Key Takeaways

- Ephemeral namespaces (`rrrt-<random>`) isolate each run and are cleaned up automatically
- RBAC is fully read-only — the analyzer never modifies cluster resources
- Signal handling ensures cleanup happens even on Ctrl+C
- Configuration flows to the Job via environment variables
- Log streaming parses structured JSON from the analyzer into user-friendly progress messages
- Path traversal validation on tar extraction protects against malicious content

Next, we'll look at the two entry points that tie everything together.
