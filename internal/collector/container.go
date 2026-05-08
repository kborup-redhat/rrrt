package collector

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/kborup-redhat/rrrt/internal/types"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *Collector) collectContainers(ctx context.Context, namespace string) ([]types.ResourceAnalysis, []types.InsufficientDataEntry) {
	var analyses []types.ResourceAnalysis
	var insufficient []types.InsufficientDataEntry

	var deployments appsv1.DeploymentList
	if err := c.k8s.List(ctx, &deployments, client.InNamespace(namespace)); err != nil {
		c.logProgress("containers", namespace, "", 0, 0, fmt.Sprintf("error listing deployments: %v", err))
		return nil, nil
	}

	var statefulSets appsv1.StatefulSetList
	if err := c.k8s.List(ctx, &statefulSets, client.InNamespace(namespace)); err != nil {
		c.logProgress("containers", namespace, "", 0, 0, fmt.Sprintf("error listing statefulsets: %v", err))
	}

	type workload struct {
		name        string
		namespace   string
		kind        types.ResourceKind
		labels      map[string]string
		annotations map[string]string
		cpuMillis   int64
		memBytes    int64
	}

	var workloads []workload
	for _, d := range deployments.Items {
		w := workload{
			name: d.Name, namespace: d.Namespace,
			kind: types.KindDeployment,
			labels: d.Labels, annotations: d.Annotations,
		}
		for _, ctr := range d.Spec.Template.Spec.Containers {
			w.cpuMillis += ctr.Resources.Requests.Cpu().MilliValue()
			w.memBytes += ctr.Resources.Requests.Memory().Value()
		}
		workloads = append(workloads, w)
	}
	for _, s := range statefulSets.Items {
		w := workload{
			name: s.Name, namespace: s.Namespace,
			kind: types.KindStatefulSet,
			labels: s.Labels, annotations: s.Annotations,
		}
		for _, ctr := range s.Spec.Template.Spec.Containers {
			w.cpuMillis += ctr.Resources.Requests.Cpu().MilliValue()
			w.memBytes += ctr.Resources.Requests.Memory().Value()
		}
		workloads = append(workloads, w)
	}

	for i, w := range workloads {
		if w.annotations[types.AnnotationExclude] == "true" {
			continue
		}
		if w.cpuMillis == 0 || w.memBytes == 0 {
			continue
		}

		c.logProgress("containers", w.namespace, w.name, i+1, len(workloads), "analyzing")

		lookback := fmt.Sprintf("%dd", c.lookbackDays)
		step := queryStep(c.lookbackDays)

		cpuSamples, err := c.prom.Query(ctx, containerCPUQuery(w.name, w.namespace, lookback, step))
		if err != nil {
			continue
		}
		memSamples, err := c.prom.Query(ctx, containerMemoryQuery(w.name, w.namespace, lookback))
		if err != nil {
			continue
		}

		var cpuVals, memVals []float64
		for _, s := range cpuSamples {
			cpuVals = append(cpuVals, s.Values...)
		}
		for _, s := range memSamples {
			memVals = append(memVals, s.Values...)
		}

		stepMin := queryStepMinutes(c.lookbackDays)
		expectedPoints := c.lookbackDays * 24 * 60 / stepMin
		minPoints := 7 * 24 * 60 / stepMin
		if len(cpuVals) < minPoints {
			insufficient = append(insufficient, types.InsufficientDataEntry{
				Namespace: w.namespace, Name: w.name, Kind: w.kind,
				DataPoints: len(cpuVals), ExpectedPoints: expectedPoints,
			})
			continue
		}

		cpuP95Raw := calculator.ComputePercentile(cpuVals, 95)
		memP95Raw := calculator.ComputePercentile(memVals, 95)
		cpuMaxRaw := maxVal(cpuVals)
		memMaxRaw := maxVal(memVals)

		cpuP95 := cpuP95Raw / (float64(w.cpuMillis) / 1000) * 100
		memP95 := memP95Raw / float64(w.memBytes) * 100
		cpuMax := cpuMaxRaw / (float64(w.cpuMillis) / 1000) * 100
		memMax := memMaxRaw / float64(w.memBytes) * 100

		result := calculator.Analyze(calculator.AnalysisInput{
			CurrentCPU:         w.cpuMillis,
			CurrentMem:         w.memBytes,
			CPUP95Percent:      cpuP95,
			MemP95Percent:      memP95,
			CPUMaxPercent:      cpuMax,
			MemMaxPercent:      memMax,
			HeadroomPercent:    c.headroomPct,
			MinCPUSavings:      types.DefaultContainerMinCPUSavings,
			MinMemSavings:      types.DefaultContainerMinMemSavings,
			UpsizeThresholdPct: types.DefaultUpsizeThreshold,
			LookbackDays:       c.lookbackDays,
		})

		ownerStr, _ := c.owner.ResolveFromLabels(ctx, w.labels, w.namespace)

		var consoleURL string
		if w.kind == types.KindDeployment {
			consoleURL = fmt.Sprintf("%s/k8s/ns/%s/deployments/%s", c.consoleURL, w.namespace, w.name)
		} else {
			consoleURL = fmt.Sprintf("%s/k8s/ns/%s/statefulsets/%s", c.consoleURL, w.namespace, w.name)
		}

		analysis := types.ResourceAnalysis{
			Namespace: w.namespace, Name: w.name, Kind: w.kind,
			Owner: ownerStr, ConsoleURL: consoleURL,
			CurrentCPU: w.cpuMillis, CurrentMem: w.memBytes,
			CPUP95: cpuP95, MemP95: memP95,
			CPUMax: cpuMax, MemMax: memMax,
		}

		if result != nil {
			analysis.Direction = result.Direction
			analysis.RecommendedCPU = result.RecommendedCPU
			analysis.RecommendedMem = result.RecommendedMem
			analysis.CPUSavings = result.CPUSavings
			analysis.MemSavings = result.MemSavings
			analysis.Justification = result.Reason
		}

		analyses = append(analyses, analysis)
	}

	return analyses, insufficient
}
