package collector

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/types"
)

func (c *Collector) collectClusterOverview(ctx context.Context) *types.ClusterOverview {
	overview := &types.ClusterOverview{}

	c.logProgress("cluster", "", "", 0, 0, "collecting cluster overview")

	overview.TotalNodes = int(c.queryScalar(ctx, clusterNodeCountQuery()))
	overview.ReadyNodes = int(c.queryScalar(ctx, clusterReadyNodesQuery()))
	overview.MasterNodes = int(c.queryScalar(ctx, clusterControlPlaneNodesQuery()))
	overview.WorkerNodes = int(c.queryScalar(ctx, clusterWorkerNodesQuery()))

	cpuCapCores := c.queryScalar(ctx, clusterCPUCapacityQuery())
	overview.CPUCapacity = int64(cpuCapCores * 1000)
	overview.CPUAllocatable = overview.CPUCapacity

	cpuReqCores := c.queryScalar(ctx, clusterCPURequestedQuery())
	overview.CPURequested = int64(cpuReqCores * 1000)

	cpuUsedCores := c.queryScalar(ctx, clusterCPUUsedQuery())
	overview.CPUUsed = int64(cpuUsedCores * 1000)

	overview.MemCapacity = int64(c.queryScalar(ctx, clusterMemCapacityQuery()))
	overview.MemAllocatable = overview.MemCapacity
	overview.MemRequested = int64(c.queryScalar(ctx, clusterMemRequestedQuery()))
	overview.MemUsed = int64(c.queryScalar(ctx, clusterMemUsedQuery()))

	overview.StorageRequested = int64(c.queryScalar(ctx, clusterStorageRequestedQuery()))
	overview.StorageCapacity = int64(c.queryScalar(ctx, clusterStorageCapacityQuery()))

	return overview
}

func (c *Collector) queryScalar(ctx context.Context, query string) float64 {
	samples, err := c.clusterProm.Query(ctx, query)
	if err != nil {
		c.logProgress("cluster", "", "", 0, 0, fmt.Sprintf("query failed: %s: %v", query, err))
		return 0
	}
	if len(samples) == 0 || len(samples[0].Values) == 0 {
		return 0
	}
	return samples[0].Values[0]
}
