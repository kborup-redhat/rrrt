package pdf

import (
	"bytes"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorRed   = drawing.Color{R: 204, G: 0, B: 0, A: 255}     // RH Dark Red
	colorGreen = drawing.Color{R: 63, G: 156, B: 53, A: 255}    // RH Green
	colorAmber = drawing.Color{R: 240, G: 171, B: 0, A: 255}    // RH Gold
)

func renderDonutChart(rightSized, oversized, undersized int, width, height int) ([]byte, error) {
	pie := chart.DonutChart{
		Width:  width,
		Height: height,
		Values: []chart.Value{
			{Label: "Right-sized", Value: float64(rightSized), Style: chart.Style{FillColor: colorGreen}},
			{Label: "Oversized", Value: float64(oversized), Style: chart.Style{FillColor: colorAmber}},
			{Label: "Undersized", Value: float64(undersized), Style: chart.Style{FillColor: colorRed}},
		},
	}

	var buf bytes.Buffer
	if err := pie.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
