# RRRT — Resource Rightsizing Reporting Tool

> **POC / Community Project** — This tool is not supported by Red Hat in any way.

An `oc`/`kubectl` CLI plugin that generates professional PDF rightsizing reports for KubeVirt VirtualMachines and container workloads (Deployments, StatefulSets) on OpenShift clusters.

## How It Works

1. You run `oc rrrt report` from your terminal
2. The CLI creates a temporary namespace on your cluster
3. An analysis Job runs inside the cluster with direct Prometheus access
4. The Job collects CPU/memory metrics, runs a rightsizing calculator, and generates a PDF
5. The CLI downloads the PDF and cleans up the temporary namespace

## Installation

Download the latest binary from [GitHub Releases](https://github.com/kborup-redhat/rrrt/releases) and place it in your PATH:

```bash
# Linux amd64
curl -L https://github.com/kborup-redhat/rrrt/releases/latest/download/oc-rrrt-linux-amd64 -o /usr/local/bin/oc-rrrt
chmod +x /usr/local/bin/oc-rrrt
```

## Usage

```bash
# Analyze all namespaces (excludes openshift-* and kube-* by default)
oc rrrt report

# Analyze specific namespaces
oc rrrt report -n production -n staging

# Custom output path and lookback window
oc rrrt report -o my-report.pdf --lookback-days 30

# Include OpenShift system namespaces
oc rrrt report --include-openshift
```

## Requirements

- OpenShift cluster with Prometheus/Thanos monitoring
- `cluster-admin` role (needed to create namespaces, ClusterRoleBindings, and Jobs)
- The `quay.io/kborup/rrrt` analyzer image must be pullable from the cluster

## Report Contents

- **Cover page** with cluster name, scope, and generation timestamp
- **Executive summary** with total savings and distribution chart
- **VM section** with per-VM metrics tables, utilization charts, and rightsizing justification
- **Container section** with per-workload metrics tables, charts, and justification
- **Insufficient data** section listing resources without enough metrics
- **Appendix** with analysis settings and version info

## License

Apache License 2.0
