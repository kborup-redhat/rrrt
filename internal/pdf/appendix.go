package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderAppendix(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	sectionHeader(p, "Appendix")
	p.Ln(5)

	y := settingsCard(p, "Analysis Settings", 15, p.GetY(), 180, [][2]string{
		{"Percentile", fmt.Sprintf("P%d", data.Percentile)},
		{"Headroom", fmt.Sprintf("%d%%", data.HeadroomPct)},
		{"Lookback", fmt.Sprintf("%d days", data.LookbackDays)},
		{"VM Min CPU Savings", formatCPU(int64(types.DefaultVMMinCPUSavings))},
		{"VM Min Memory Savings", formatMem(int64(types.DefaultVMMinMemSavings))},
		{"Container Min CPU Savings", formatCPU(int64(types.DefaultContainerMinCPUSavings))},
		{"Container Min Memory Savings", formatMem(int64(types.DefaultContainerMinMemSavings))},
		{"Upsize Threshold", fmt.Sprintf("%d%%", types.DefaultUpsizeThreshold)},
	})

	versionRows := [][2]string{
		{"CLI Version", data.CLIVersion},
		{"Analyzer Image", data.ImageVersion},
	}
	if data.ClusterID != "" {
		versionRows = append(versionRows, [2]string{"Cluster ID", data.ClusterID})
	}

	settingsCard(p, "Version Information", 15, y+8, 180, versionRows)

	p.SetY(-35)
	p.SetFont("Helvetica", "", 9)
	setText(p, clrSubtext)
	p.CellFormat(0, 5, "Built with RRRT open source project", "", 1, "C", false, 0, "")
	setText(p, clrBlue)
	p.CellFormat(0, 5, "github.com/kborup-redhat/rrrt", "", 1, "C", false, 0, "https://github.com/kborup-redhat/rrrt")
	setText(p, clrDarkText)
}
