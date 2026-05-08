package collector

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/calculator"
	"github.com/kborup-redhat/rrrt/internal/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *Collector) collectVMs(ctx context.Context, namespace string) ([]types.ResourceAnalysis, []types.InsufficientDataEntry) {
	var analyses []types.ResourceAnalysis
	var insufficient []types.InsufficientDataEntry

	vmList := &unstructured.UnstructuredList{}
	vmList.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineList"})

	opts := []client.ListOption{client.InNamespace(namespace)}
	if err := c.k8s.List(ctx, vmList, opts...); err != nil {
		c.logProgress("vms", namespace, "", 0, 0, fmt.Sprintf("error listing VMs: %v", err))
		return nil, nil
	}

	for i, vm := range vmList.Items {
		name := vm.GetName()
		ns := vm.GetNamespace()

		annotations := vm.GetAnnotations()
		if annotations[types.AnnotationExclude] == "true" {
			continue
		}

		c.logProgress("vms", ns, name, i+1, len(vmList.Items), "analyzing")

		cores, _, _ := unstructured.NestedInt64(vm.Object, "spec", "template", "spec", "domain", "cpu", "cores")
		memStr, _, _ := unstructured.NestedString(vm.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")

		cpuMillis := cores * 1000
		var memBytes int64
		if memStr != "" {
			q, err := resource.ParseQuantity(memStr)
			if err == nil {
				memBytes = q.Value()
			}
		}

		lookback := fmt.Sprintf("%dd", c.lookbackDays)

		cpuSamples, err := c.prom.Query(ctx, vmCPUQuery(name, ns, lookback))
		if err != nil {
			c.logProgress("vms", ns, name, i+1, len(vmList.Items), fmt.Sprintf("error querying CPU: %v", err))
			continue
		}
		memSamples, err := c.prom.Query(ctx, vmMemoryQuery(name, ns, lookback))
		if err != nil {
			c.logProgress("vms", ns, name, i+1, len(vmList.Items), fmt.Sprintf("error querying memory: %v", err))
			continue
		}

		var cpuVals, memVals []float64
		if len(cpuSamples) > 0 {
			cpuVals = cpuSamples[0].Values
		}
		if len(memSamples) > 0 {
			memVals = memSamples[0].Values
		}

		expectedPoints := c.lookbackDays * 24 * 60
		minPoints := expectedPoints / 2
		if len(cpuVals) < minPoints {
			insufficient = append(insufficient, types.InsufficientDataEntry{
				Namespace: ns, Name: name, Kind: types.KindVM,
				DataPoints: len(cpuVals), ExpectedPoints: expectedPoints,
			})
			continue
		}

		cpuP95 := calculator.ComputePercentile(cpuVals, 95) * 100
		memP95 := calculator.ComputePercentile(memVals, 95)
		cpuMax := maxVal(cpuVals) * 100
		memMax := maxVal(memVals)

		if memBytes > 0 {
			memP95 = memP95 / float64(memBytes) * 100
			memMax = memMax / float64(memBytes) * 100
		}

		result := calculator.Analyze(calculator.AnalysisInput{
			CurrentCPU:         cpuMillis,
			CurrentMem:         memBytes,
			CPUP95Percent:      cpuP95,
			MemP95Percent:      memP95,
			CPUMaxPercent:      cpuMax,
			MemMaxPercent:      memMax,
			HeadroomPercent:    c.headroomPct,
			MinCPUSavings:      types.DefaultVMMinCPUSavings,
			MinMemSavings:      types.DefaultVMMinMemSavings,
			UpsizeThresholdPct: types.DefaultUpsizeThreshold,
			LookbackDays:       c.lookbackDays,
		})

		ownerStr, _ := c.owner.ResolveFromLabels(ctx, vm.GetLabels(), ns)

		consoleURL := fmt.Sprintf("%s/k8s/ns/%s/kubevirt.io~v1~VirtualMachine/%s", c.consoleURL, ns, name)

		analysis := types.ResourceAnalysis{
			Namespace: ns, Name: name, Kind: types.KindVM,
			Owner: ownerStr, ConsoleURL: consoleURL,
			CurrentCPU: cpuMillis, CurrentMem: memBytes,
			CPUP95: cpuP95, MemP95: memP95,
			CPUMax: cpuMax, MemMax: memMax,
			CPUSamples: cpuVals, MemSamples: memVals,
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

func maxVal(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

