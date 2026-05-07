package pdf

import (
	"fmt"

	"github.com/go-pdf/fpdf"
)

var (
	clrNavy      = [3]int{21, 21, 21}       // RH Black #151515
	clrBlue      = [3]int{238, 0, 0}        // RH Red #EE0000
	clrGreen     = [3]int{63, 156, 53}      // RH Green #3F9C35
	clrAmber     = [3]int{240, 171, 0}      // RH Gold #F0AB00
	clrAmberDark = [3]int{196, 144, 0}
	clrRed       = [3]int{204, 0, 0}        // RH Dark Red #CC0000
	clrLightGrey = [3]int{240, 240, 240}    // RH Light Gray #F0F0F0
	clrMidGrey   = [3]int{210, 210, 210}    // RH Mid Gray #D2D2D2
	clrDarkText  = [3]int{21, 21, 21}       // RH Black #151515
	clrWhite     = [3]int{255, 255, 255}
	clrSubtext   = [3]int{106, 110, 115}    // RH Gray #6A6E73
)

func setFill(p *fpdf.Fpdf, c [3]int) {
	p.SetFillColor(c[0], c[1], c[2])
}

func setDraw(p *fpdf.Fpdf, c [3]int) {
	p.SetDrawColor(c[0], c[1], c[2])
}

func setText(p *fpdf.Fpdf, c [3]int) {
	p.SetTextColor(c[0], c[1], c[2])
}

func sectionHeader(p *fpdf.Fpdf, title string) {
	setFill(p, clrNavy)
	p.Rect(10, p.GetY(), 3, 10, "F")

	p.SetX(17)
	p.SetFont("Helvetica", "B", 18)
	setText(p, clrNavy)
	p.CellFormat(0, 10, title, "", 1, "L", false, 0, "")

	y := p.GetY() + 1
	setDraw(p, clrBlue)
	p.SetLineWidth(0.5)
	p.Line(10, y, 200, y)
	p.SetLineWidth(0.2)
	p.SetY(y + 4)

	setText(p, clrDarkText)
}

func pageFooter(p *fpdf.Fpdf) {
	p.SetFooterFunc(func() {
		p.SetY(-15)
		setDraw(p, clrMidGrey)
		p.SetLineWidth(0.3)
		p.Line(10, p.GetY(), 200, p.GetY())
		p.SetLineWidth(0.2)
		p.Ln(2)

		p.SetFont("Helvetica", "", 8)
		setText(p, clrSubtext)
		p.CellFormat(95, 5, "RRRT - Resource Rightsizing Report", "", 0, "L", false, 0, "")
		p.CellFormat(95, 5, fmt.Sprintf("Page %d", p.PageNo()), "", 0, "R", false, 0, "")
		setText(p, clrDarkText)
	})
}

func statCard(p *fpdf.Fpdf, x, y, w, h float64, bg [3]int, value, label string) {
	setFill(p, bg)
	p.RoundedRect(x, y, w, h, 2, "1234", "F")

	setText(p, clrWhite)
	p.SetFont("Helvetica", "B", 22)
	p.SetXY(x, y+6)
	p.CellFormat(w, 10, value, "", 1, "C", false, 0, "")

	p.SetFont("Helvetica", "", 9)
	p.SetX(x)
	p.CellFormat(w, 6, label, "", 1, "C", false, 0, "")

	setText(p, clrDarkText)
}

func infoCard(p *fpdf.Fpdf, x, y, w float64, rows [][2]string) {
	rowH := 7.0
	h := float64(len(rows))*rowH + 6

	setFill(p, clrLightGrey)
	p.RoundedRect(x, y, w, h, 2, "1234", "F")

	setFill(p, clrBlue)
	p.Rect(x, y, 3, h, "F")

	p.SetFont("Helvetica", "", 10)
	for i, row := range rows {
		ry := y + 3 + float64(i)*rowH
		setText(p, clrSubtext)
		p.SetXY(x+8, ry)
		p.CellFormat(40, rowH, row[0], "", 0, "L", false, 0, "")
		setText(p, clrDarkText)
		p.CellFormat(w-52, rowH, row[1], "", 0, "L", false, 0, "")
	}
}

func settingsCard(p *fpdf.Fpdf, title string, x, y, w float64, rows [][2]string) float64 {
	rowH := 6.5
	headerH := 8.0
	padding := 4.0
	h := headerH + float64(len(rows))*rowH + padding*2

	setFill(p, clrLightGrey)
	p.RoundedRect(x, y, w, h, 2, "1234", "F")

	setFill(p, clrNavy)
	p.RoundedRect(x, y, w, headerH, 2, "12", "F")
	setText(p, clrWhite)
	p.SetFont("Helvetica", "B", 10)
	p.SetXY(x+5, y)
	p.CellFormat(w-10, headerH, title, "", 0, "L", false, 0, "")

	p.SetFont("Helvetica", "", 9)
	for i, row := range rows {
		ry := y + headerH + padding + float64(i)*rowH

		if i%2 == 0 {
			setFill(p, clrWhite)
			p.Rect(x+1, ry, w-2, rowH, "F")
		}

		setText(p, clrSubtext)
		p.SetXY(x+5, ry)
		p.CellFormat(55, rowH, row[0], "", 0, "L", false, 0, "")
		setText(p, clrDarkText)
		p.CellFormat(w-65, rowH, row[1], "", 0, "L", false, 0, "")
	}

	setText(p, clrDarkText)
	return y + h
}
