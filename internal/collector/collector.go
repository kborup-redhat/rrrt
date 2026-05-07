package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Collector orchestrates metric collection for VMs and containers.
type Collector struct {
	k8s          client.Client
	prom         *PrometheusClient
	owner        *owner.Resolver
	consoleURL   string
	lookbackDays int
	headroomPct  int
	includeOS    bool
}

// New creates a Collector with the given configuration.
func New(k8s client.Client, prom *PrometheusClient, ownerResolver *owner.Resolver, consoleURL string, lookbackDays, headroomPct int, includeOpenShift bool) *Collector {
	return &Collector{
		k8s: k8s, prom: prom, owner: ownerResolver,
		consoleURL: consoleURL, lookbackDays: lookbackDays,
		headroomPct: headroomPct, includeOS: includeOpenShift,
	}
}

// Collect gathers metrics across the specified namespaces (or all namespaces
// if none are supplied) and returns a populated ReportData.
func (c *Collector) Collect(ctx context.Context, namespaces []string) (*types.ReportData, error) {
	if len(namespaces) == 0 {
		var nsList corev1.NamespaceList
		if err := c.k8s.List(ctx, &nsList); err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	namespaces = c.filterNamespaces(namespaces)

	data := &types.ReportData{
		LookbackDays: c.lookbackDays,
		Percentile:   types.DefaultPercentile,
		HeadroomPct:  c.headroomPct,
	}

	total := len(namespaces)
	for i, ns := range namespaces {
		c.logProgress("scan", ns, "", i+1, total, "scanning namespace")

		vms, vmInsuf := c.collectVMs(ctx, ns)
		data.VMAnalyses = append(data.VMAnalyses, vms...)
		data.InsufficientData = append(data.InsufficientData, vmInsuf...)

		containers, contInsuf := c.collectContainers(ctx, ns)
		data.ContainerAnalyses = append(data.ContainerAnalyses, containers...)
		data.InsufficientData = append(data.InsufficientData, contInsuf...)
	}

	return data, nil
}

func (c *Collector) filterNamespaces(namespaces []string) []string {
	if c.includeOS {
		return namespaces
	}
	var filtered []string
	for _, ns := range namespaces {
		if strings.HasPrefix(ns, "openshift-") ||
			strings.HasPrefix(ns, "kube-") ||
			strings.HasPrefix(ns, "rrrt-") {
			continue
		}
		filtered = append(filtered, ns)
	}
	return filtered
}

func (c *Collector) logProgress(phase, namespace, resource string, index, total int, status string) {
	msg := map[string]interface{}{
		"phase":     phase,
		"namespace": namespace,
		"resource":  resource,
		"index":     index,
		"total":     total,
		"status":    status,
	}
	data, _ := json.Marshal(msg)
	fmt.Fprintln(os.Stdout, string(data))
}
