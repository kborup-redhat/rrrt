package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderClusterOverview(p *fpdf.Fpdf, data *types.ReportData) {
	if data.ClusterOverview == nil {
		return
	}
	ov := data.ClusterOverview

	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Cluster Overview", "", 1, "L", false, 0, "")
	p.Ln(5)

	p.SetFont("Helvetica", "", 11)
	nodeStatus := "all Ready"
	if ov.ReadyNodes < ov.TotalNodes {
		nodeStatus = fmt.Sprintf("%d Ready", ov.ReadyNodes)
	}
	p.CellFormat(0, 7, fmt.Sprintf("Nodes: %d (%d control plane, %d worker) — %s",
		ov.TotalNodes, ov.MasterNodes, ov.WorkerNodes, nodeStatus), "", 1, "L", false, 0, "")
	p.Ln(8)

	barX := 20.0
	barW := 170.0
	barH := 12.0

	if ov.CPUCapacity > 0 {
		usedPct := pct(ov.CPUUsed, ov.CPUCapacity)
		reqPct := pct(ov.CPURequested, ov.CPUCapacity)
		drawGaugeSection(p, "CPU", barX, barW, barH,
			usedPct, reqPct,
			fmt.Sprintf("Used: %s (%.0f%%)", formatCPU(ov.CPUUsed), usedPct),
			fmt.Sprintf("Requested: %s (%.0f%%)", formatCPU(ov.CPURequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatCPU(ov.CPUCapacity)),
		)
	}

	if ov.MemCapacity > 0 {
		usedPct := pct(ov.MemUsed, ov.MemCapacity)
		reqPct := pct(ov.MemRequested, ov.MemCapacity)
		drawGaugeSection(p, "Memory", barX, barW, barH,
			usedPct, reqPct,
			fmt.Sprintf("Used: %s (%.0f%%)", formatMem(ov.MemUsed), usedPct),
			fmt.Sprintf("Requested: %s (%.0f%%)", formatMem(ov.MemRequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatMem(ov.MemCapacity)),
		)
	}

	if ov.StorageCapacity > 0 {
		reqPct := pct(ov.StorageRequested, ov.StorageCapacity)
		drawGaugeSection(p, "Storage (PVC)", barX, barW, barH,
			0, reqPct,
			"",
			fmt.Sprintf("Requested: %s (%.0f%%)", formatMem(ov.StorageRequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatMem(ov.StorageCapacity)),
		)
	}
}

func drawGaugeSection(p *fpdf.Fpdf, title string, x, w, h float64, usedPct, reqPct float64, usedLabel, reqLabel, capLabel string) {
	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	p.Ln(2)

	y := p.GetY()

	// Background (free/grey)
	p.SetFillColor(230, 230, 230)
	p.Rect(x, y, w, h, "F")

	// Requested portion (amber) — drawn first so used overlaps
	if reqPct > 0 {
		rw := w * clamp(reqPct, 0, 100) / 100
		p.SetFillColor(251, 188, 4)
		p.Rect(x, y, rw, h, "F")
	}

	// Used portion (blue)
	if usedPct > 0 {
		uw := w * clamp(usedPct, 0, 100) / 100
		p.SetFillColor(66, 133, 244)
		p.Rect(x, y, uw, h, "F")
	}

	// Border
	p.SetDrawColor(180, 180, 180)
	p.Rect(x, y, w, h, "D")

	p.SetY(y + h + 3)
	p.SetFont("Helvetica", "", 9)

	if usedLabel != "" {
		p.SetTextColor(66, 133, 244)
		p.CellFormat(60, 5, usedLabel, "", 0, "L", false, 0, "")
	} else {
		p.CellFormat(60, 5, "", "", 0, "L", false, 0, "")
	}
	p.SetTextColor(180, 130, 0)
	p.CellFormat(60, 5, reqLabel, "", 0, "L", false, 0, "")
	p.SetTextColor(100, 100, 100)
	p.CellFormat(0, 5, capLabel, "", 1, "L", false, 0, "")

	p.SetTextColor(0, 0, 0)
	p.Ln(10)
}

func pct(value, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
