package pdf

import (
	"bytes"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorBlue  = drawing.Color{R: 238, G: 0, B: 0, A: 255}     // RH Red
	colorRed   = drawing.Color{R: 204, G: 0, B: 0, A: 255}     // RH Dark Red
	colorGrey  = drawing.Color{R: 106, G: 110, B: 115, A: 255}  // RH Gray
	colorGreen = drawing.Color{R: 63, G: 156, B: 53, A: 255}    // RH Green
	colorAmber = drawing.Color{R: 240, G: 171, B: 0, A: 255}    // RH Gold
)

func downsample(data []float64, maxPoints int) []float64 {
	if len(data) <= maxPoints {
		return data
	}
	result := make([]float64, maxPoints)
	bucketSize := float64(len(data)) / float64(maxPoints)
	for i := 0; i < maxPoints; i++ {
		start := int(float64(i) * bucketSize)
		end := int(float64(i+1) * bucketSize)
		if end > len(data) {
			end = len(data)
		}
		max := data[start]
		for j := start + 1; j < end; j++ {
			if data[j] > max {
				max = data[j]
			}
		}
		result[i] = max
	}
	return result
}

func renderLineChart(samples []float64, p95 float64, currentAllocation float64, title string, width, height int) ([]byte, error) {
	samples = downsample(samples, width)
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
