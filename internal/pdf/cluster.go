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
	sectionHeader(p, "Cluster Overview")
	p.Ln(3)

	nodeStatus := "all Ready"
	if ov.ReadyNodes < ov.TotalNodes {
		nodeStatus = fmt.Sprintf("%d Ready", ov.ReadyNodes)
	}

	boxY := p.GetY()
	setFill(p, [3]int{230, 243, 255})
	p.RoundedRect(15, boxY, 180, 14, 2, "1234", "F")
	setFill(p, clrBlue)
	p.Rect(15, boxY, 3, 14, "F")

	p.SetFont("Helvetica", "B", 11)
	setText(p, clrNavy)
	p.SetXY(22, boxY+1)
	p.CellFormat(170, 6, fmt.Sprintf("%d Nodes (%d control plane, %d worker)",
		ov.TotalNodes, ov.MasterNodes, ov.WorkerNodes), "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	p.SetX(22)
	p.CellFormat(170, 5, nodeStatus, "", 1, "L", false, 0, "")
	setText(p, clrDarkText)

	p.SetY(boxY + 22)

	barX := 15.0
	barW := 180.0
	barH := 14.0

	if ov.CPUCapacity > 0 {
		usedPct := pct(ov.CPUUsed, ov.CPUCapacity)
		reqPct := pct(ov.CPURequested, ov.CPUCapacity)
		drawStyledGauge(p, "CPU", barX, barW, barH,
			usedPct, reqPct,
			fmt.Sprintf("Used: %s (%.0f%%)", formatCPU(ov.CPUUsed), usedPct),
			fmt.Sprintf("Requested: %s (%.0f%%)", formatCPU(ov.CPURequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatCPU(ov.CPUCapacity)),
		)
	}

	if ov.MemCapacity > 0 {
		usedPct := pct(ov.MemUsed, ov.MemCapacity)
		reqPct := pct(ov.MemRequested, ov.MemCapacity)
		drawStyledGauge(p, "Memory", barX, barW, barH,
			usedPct, reqPct,
			fmt.Sprintf("Used: %s (%.0f%%)", formatMem(ov.MemUsed), usedPct),
			fmt.Sprintf("Requested: %s (%.0f%%)", formatMem(ov.MemRequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatMem(ov.MemCapacity)),
		)
	}

	if ov.StorageCapacity > 0 {
		reqPct := pct(ov.StorageRequested, ov.StorageCapacity)
		drawStyledGauge(p, "Storage (PVC)", barX, barW, barH,
			0, reqPct,
			"",
			fmt.Sprintf("Requested: %s (%.0f%%)", formatMem(ov.StorageRequested), reqPct),
			fmt.Sprintf("Capacity: %s", formatMem(ov.StorageCapacity)),
		)
	}
}

func drawStyledGauge(p *fpdf.Fpdf, title string, x, w, h float64, usedPct, reqPct float64, usedLabel, reqLabel, capLabel string) {
	p.SetFont("Helvetica", "B", 11)
	setText(p, clrNavy)
	p.CellFormat(0, 7, title, "", 1, "L", false, 0, "")
	setText(p, clrDarkText)
	p.Ln(1)

	y := p.GetY()

	setFill(p, [3]int{230, 230, 230})
	p.RoundedRect(x, y, w, h, 2, "1234", "F")

	if reqPct > 0 {
		rw := w * clamp(reqPct, 0, 100) / 100
		setFill(p, clrAmber)
		p.RoundedRect(x, y, rw, h, 2, "1234", "F")
	}

	if usedPct > 0 {
		uw := w * clamp(usedPct, 0, 100) / 100
		setFill(p, clrBlue)
		p.RoundedRect(x, y, uw, h, 2, "1234", "F")
	}

	setDraw(p, clrMidGrey)
	p.RoundedRect(x, y, w, h, 2, "1234", "D")

	if usedPct > 5 {
		setText(p, clrWhite)
		p.SetFont("Helvetica", "B", 9)
		p.SetXY(x+3, y+2)
		p.CellFormat(40, h-4, fmt.Sprintf("%.0f%%", usedPct), "", 0, "L", false, 0, "")
	}

	p.SetY(y + h + 3)
	p.SetFont("Helvetica", "", 9)

	if usedLabel != "" {
		setText(p, clrBlue)
		p.CellFormat(60, 5, usedLabel, "", 0, "L", false, 0, "")
	} else {
		p.CellFormat(60, 5, "", "", 0, "L", false, 0, "")
	}
	setText(p, clrAmberDark)
	p.CellFormat(60, 5, reqLabel, "", 0, "L", false, 0, "")
	setText(p, clrSubtext)
	p.CellFormat(0, 5, capLabel, "", 1, "L", false, 0, "")

	setText(p, clrDarkText)
	p.Ln(8)
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
