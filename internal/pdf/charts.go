package pdf

import (
	"bytes"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorBlue  = drawing.Color{R: 66, G: 133, B: 244, A: 255}
	colorRed   = drawing.Color{R: 234, G: 67, B: 53, A: 255}
	colorGrey  = drawing.Color{R: 158, G: 158, B: 158, A: 255}
	colorGreen = drawing.Color{R: 52, G: 168, B: 83, A: 255}
	colorAmber = drawing.Color{R: 251, G: 188, B: 4, A: 255}
)

func renderLineChart(samples []float64, p95 float64, currentAllocation float64, title string, width, height int) ([]byte, error) {
	xValues := make([]float64, len(samples))
	for i := range samples {
		xValues[i] = float64(i)
	}

	graph := chart.Chart{
		Title:  title,
		Width:  width,
		Height: height,
		Background: chart.Style{
			FillColor: drawing.Color{R: 255, G: 255, B: 255, A: 255},
		},
		XAxis: chart.XAxis{
			Name: "Time",
			Style: chart.Style{
				FontSize: 8,
			},
		},
		YAxis: chart.YAxis{
			Name: "Utilization %",
			Range: &chart.ContinuousRange{Min: 0, Max: 100},
			Style: chart.Style{
				FontSize: 8,
			},
		},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Name:    "Utilization",
				XValues: xValues,
				YValues: samples,
				Style: chart.Style{
					StrokeColor: colorBlue,
					StrokeWidth: 1.5,
					FillColor:   colorBlue.WithAlpha(40),
				},
			},
			chart.ContinuousSeries{
				Name:    "P95",
				XValues: []float64{0, float64(len(samples) - 1)},
				YValues: []float64{p95, p95},
				Style: chart.Style{
					StrokeColor:     colorRed,
					StrokeWidth:     2,
					StrokeDashArray: []float64{5, 3},
				},
			},
			chart.ContinuousSeries{
				Name:    "Current",
				XValues: []float64{0, float64(len(samples) - 1)},
				YValues: []float64{currentAllocation, currentAllocation},
				Style: chart.Style{
					StrokeColor:     colorGrey,
					StrokeWidth:     1.5,
					StrokeDashArray: []float64{3, 3},
				},
			},
		},
	}

	graph.Elements = []chart.Renderable{chart.LegendThin(&graph)}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

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
