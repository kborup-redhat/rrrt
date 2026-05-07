package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderAppendix(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()
	p.SetFont("Helvetica", "B", 16)
	p.CellFormat(0, 10, "Appendix", "", 1, "L", false, 0, "")
	p.Ln(5)

	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(0, 8, "Analysis Settings", "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	p.CellFormat(0, 6, fmt.Sprintf("Percentile: P%d", data.Percentile), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Headroom: %d%%", data.HeadroomPct), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Lookback: %d days", data.LookbackDays), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("VM Min CPU Savings: %s", formatCPU(int64(types.DefaultVMMinCPUSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("VM Min Memory Savings: %s", formatMem(int64(types.DefaultVMMinMemSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Container Min CPU Savings: %s", formatCPU(int64(types.DefaultContainerMinCPUSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Container Min Memory Savings: %s", formatMem(int64(types.DefaultContainerMinMemSavings))), "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, fmt.Sprintf("Upsize Threshold: %d%%", types.DefaultUpsizeThreshold), "", 1, "L", false, 0, "")

	p.Ln(10)
	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(0, 8, "Version Information", "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 10)
	p.CellFormat(0, 6, "CLI Version: "+data.CLIVersion, "", 1, "L", false, 0, "")
	p.CellFormat(0, 6, "Analyzer Image: "+data.ImageVersion, "", 1, "L", false, 0, "")

	p.Ln(20)
	p.SetFont("Helvetica", "", 9)
	p.SetTextColor(100, 100, 100)
	p.CellFormat(0, 5, "Built with RRRT open source project — github.com/kborup-redhat/rrrt", "", 1, "C", false, 0, "https://github.com/kborup-redhat/rrrt")
	p.CellFormat(0, 5, "Feedback & issues: github.com/kborup-redhat/rrrt/issues", "", 1, "C", false, 0, "https://github.com/kborup-redhat/rrrt/issues")
	p.SetTextColor(0, 0, 0)
}
