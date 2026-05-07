package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func renderInsufficientData(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.InsufficientData) == 0 {
		return
	}

	p.AddPage()
	sectionHeader(p, "Insufficient Data")
	p.Ln(3)

	p.SetFont("Helvetica", "", 9)
	setText(p, clrSubtext)
	p.MultiCell(0, 5, "The following resources had less than 50% metric coverage of the lookback window and could not be analyzed.", "", "L", false)
	setText(p, clrDarkText)
	p.Ln(5)

	headers := []string{"Namespace", "Name", "Type", "Data Points", "Expected"}
	widths := []float64{35, 40, 30, 35, 35}

	p.SetFont("Helvetica", "B", 8)
	setFill(p, clrNavy)
	setText(p, clrWhite)
	for i, h := range headers {
		p.CellFormat(widths[i], 7, h, "", 0, "C", true, 0, "")
	}
	p.Ln(-1)

	p.SetFont("Helvetica", "", 8)
	setText(p, clrDarkText)
	for row, entry := range data.InsufficientData {
		if row%2 == 0 {
			setFill(p, clrLightGrey)
		} else {
			setFill(p, clrWhite)
		}
		p.CellFormat(widths[0], 5.5, entry.Namespace, "", 0, "L", true, 0, "")
		p.CellFormat(widths[1], 5.5, entry.Name, "", 0, "L", true, 0, "")
		p.CellFormat(widths[2], 5.5, string(entry.Kind), "", 0, "L", true, 0, "")
		p.CellFormat(widths[3], 5.5, fmt.Sprintf("%d", entry.DataPoints), "", 0, "R", true, 0, "")
		p.CellFormat(widths[4], 5.5, fmt.Sprintf("%d", entry.ExpectedPoints), "", 0, "R", true, 0, "")
		p.Ln(-1)
	}
}
