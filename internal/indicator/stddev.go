package indicator

import "math"

// stddevSeries uses population variance over complete trailing windows.
// Centered differences avoid cancellation when prices have a large offset.
func stddevSeries(input Series, period int) Series {
	output := invalidSeries(input.Len())
	for end := period - 1; end < input.Len(); end++ {
		mean, complete := 0.0, true
		for i := end - period + 1; i <= end; i++ {
			value, valid := input.At(i)
			if !valid {
				complete = false
				break
			}
			mean += value / float64(period)
		}
		if !complete {
			continue
		}
		variance := 0.0
		for i := end - period + 1; i <= end; i++ {
			delta := input.values[i] - mean
			variance += delta * delta / float64(period)
		}
		output.values[end] = math.Sqrt(variance)
		output.valid[end] = true
	}
	return output
}
