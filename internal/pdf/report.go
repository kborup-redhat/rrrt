package pdf

import (
	"fmt"
	"strconv"

	"github.com/go-pdf/fpdf"
	"github.com/kborup-redhat/rrrt/internal/types"
)

func Generate(data *types.ReportData, outputPath string) error {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetAutoPageBreak(true, 20)

	pageFooter(p)

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
	sectionHeader(p, "Virtual Machines")
	p.Ln(3)
	renderResourceTable(p, data.VMAnalyses, "Virtual Machines")
}

func renderContainerSection(p *fpdf.Fpdf, data *types.ReportData) {
	if len(data.ContainerAnalyses) == 0 {
		return
	}
	p.AddPage()
	sectionHeader(p, "Containers")
	p.Ln(3)
	renderResourceTable(p, data.ContainerAnalyses, "Containers")
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
