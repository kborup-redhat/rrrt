package pdf

import (
	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderCover(p *fpdf.Fpdf, data *types.ReportData) {
	p.AddPage()

	setFill(p, clrNavy)
	p.Rect(0, 0, 210, 90, "F")

	setText(p, clrWhite)
	p.SetFont("Helvetica", "B", 32)
	p.SetXY(20, 25)
	p.CellFormat(170, 14, "Resource Rightsizing", "", 1, "L", false, 0, "")
	p.SetX(20)
	p.CellFormat(170, 14, "Report", "", 1, "L", false, 0, "")

	p.SetFont("Helvetica", "", 12)
	p.SetXY(20, 65)
	p.CellFormat(170, 8, "OpenShift Cluster Analysis", "", 1, "L", false, 0, "")

	setText(p, clrDarkText)

	infoCard(p, 20, 110, 170, [][2]string{
		{"Cluster", data.ClusterName},
		{"Scope", data.Scope},
		{"Lookback", itoa(data.LookbackDays) + " days"},
		{"Data Source", data.DataSource},
		{"Generated", data.GeneratedAt},
	})

	setFill(p, clrBlue)
	p.Rect(20, 180, 60, 2, "F")

	p.SetFont("Helvetica", "", 10)
	setText(p, clrSubtext)
	p.SetXY(20, 250)
	p.CellFormat(170, 6, "RRRT "+data.CLIVersion+" - OpenShift Resource Rightsizing Tool", "", 1, "C", false, 0, "")
	setText(p, clrDarkText)
}
