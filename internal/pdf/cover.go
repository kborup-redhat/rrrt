package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderCover(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()

	p.SetFont("Helvetica", "B", 28)
	p.Ln(60)
	p.CellFormat(0, 15, "Resource Rightsizing Report", "", 1, "C", false, 0, "")

	p.Ln(20)
	p.SetFont("Helvetica", "", 14)
	p.CellFormat(0, 10, "Cluster: "+data.ClusterName, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Generated: "+data.GeneratedAt, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Scope: "+data.Scope, "", 1, "C", false, 0, "")
	p.CellFormat(0, 10, "Lookback: "+itoa(data.LookbackDays)+" days", "", 1, "C", false, 0, "")
}
