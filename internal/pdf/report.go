package pdf

import (
	"fmt"
	"strconv"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func Generate(data *types.ReportData, outputPath string) error {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetAutoPageBreak(true, 15)

	renderCover(p, data)
	renderSummary(p, data)
	renderClusterOverview(p, data)
	renderVMSection(p, data)
	renderContainerSection(p, data)
	renderInsufficientData(p, data)
	renderAppendix(p, data)

	return p.OutputFileAndClose(outputPath)
}

func renderVMSection(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.VMAnalyses) == 0 {
		return
	}
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Virtual Machines", "", 1, "L", false, 0, "")
	p.Ln(5)
	renderResourceTable(p, data.VMAnalyses, "Virtual Machines")
	renderDetailCards(p, data.VMAnalyses)
}

func renderContainerSection(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.ContainerAnalyses) == 0 {
		return
	}
	p.AddPage()
	p.SetFont("Helvetica", "B", 18)
	p.CellFormat(0, 10, "Containers", "", 1, "L", false, 0, "")
	p.Ln(5)
	renderResourceTable(p, data.ContainerAnalyses, "Containers")
	renderDetailCards(p, data.ContainerAnalyses)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func formatCPU(millis int64) string {
	if millis >= 1000 {
		cores := float64(millis) / 1000
		if cores == float64(int64(cores)) {
			return fmt.Sprintf("%d cores", int64(cores))
		}
		return fmt.Sprintf("%.1f cores", cores)
	}
	return fmt.Sprintf("%dm", millis)
}

func formatMem(bytes int64) string {
	if bytes < 0 {
		bytes = -bytes
	}
	const gi = 1 << 30
	const mi = 1 << 20
	if bytes >= gi {
		gib := float64(bytes) / float64(gi)
		if gib == float64(int64(gib)) {
			return fmt.Sprintf("%d GiB", int64(gib))
		}
		return fmt.Sprintf("%.1f GiB", gib)
	}
	return fmt.Sprintf("%d MiB", bytes/mi)
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
