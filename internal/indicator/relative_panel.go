package indicator

import "math"

// RelativePanelResult contains aligned, causal diagnostics of a log price ratio.
// Callers align both positive price inputs before computing and paginate afterward.
type RelativePanelResult struct{ Z, Smooth, Regime, Performance, Correlation Series }

func RelativePanel(primary, comparison []float64, band, smooth, regime int) (RelativePanelResult, error) {
	if smooth < 1 || regime <= band {
		return RelativePanelResult{}, ErrInvalidRelativePrices
	}
	z, err := RelativePriceZScore(primary, comparison, band)
	if err != nil {
		return RelativePanelResult{}, err
	}
	long, err := RelativePriceZScore(primary, comparison, regime)
	if err != nil {
		return RelativePanelResult{}, err
	}
	out := RelativePanelResult{Z: z, Smooth: emaSeries(z, smooth), Regime: long, Performance: invalidSeries(len(primary)), Correlation: invalidSeries(len(primary))}
	stockReturns, commodityReturns := make([]float64, len(primary)), make([]float64, len(primary))
	for i := range primary {
		if i >= 63 {
			delta := math.Log(primary[i]) - math.Log(comparison[i]) - math.Log(primary[i-63]) + math.Log(comparison[i-63])
			value := math.Expm1(delta) * 100
			if !math.IsInf(value, 0) {
				out.Performance.values[i] = value
				out.Performance.valid[i] = true
			}
		}
		if i >= 5 {
			stockReturns[i] = math.Log(primary[i]) - math.Log(primary[i-5])
			commodityReturns[i] = math.Log(comparison[i]) - math.Log(comparison[i-5])
		}
		if i < band+4 {
			continue
		}
		start := i - band + 1
		a, b := 0.0, 0.0
		for j := start; j <= i; j++ {
			a += stockReturns[j] / float64(band)
			b += commodityReturns[j] / float64(band)
		}
		covariance, va, vb := 0.0, 0.0, 0.0
		for j := start; j <= i; j++ {
			x, y := stockReturns[j]-a, commodityReturns[j]-b
			covariance += x * y
			va += x * x
			vb += y * y
		}
		if va > 0 && vb > 0 {
			out.Correlation.values[i] = math.Max(-1, math.Min(1, covariance/math.Sqrt(va*vb)))
			out.Correlation.valid[i] = true
		}
	}
	return out, nil
}
