package indicator

import "math"

// logReturnSeries converts an adjusted close series into per-bar log returns.
// The first point, invalid closes and non-positive prices yield invalid points.
func logReturnSeries(close Series) Series {
	output := invalidSeries(close.Len())
	previous := 0.0
	hasPrevious := false
	for index := 0; index < close.Len(); index++ {
		value, valid := close.At(index)
		if !valid || value <= 0 {
			hasPrevious = false
			continue
		}
		if hasPrevious {
			output.values[index] = math.Log(value / previous)
			output.valid[index] = true
		}
		previous = value
		hasPrevious = true
	}
	return output
}

// returnZScoreSeries standardizes each point against its trailing window:
// z = (value - SMA(window)) / populationStddev(window). Windows containing
// invalid points, incomplete windows and zero-variance windows stay invalid,
// matching the reference panel semantics (std <= 1e-12 suppressed).
func returnZScoreSeries(input Series, window int) Series {
	output := invalidSeries(input.Len())
	if window < 2 {
		return output
	}
	stddevs := stddevSeries(input, window)
	for end := window - 1; end < input.Len(); end++ {
		mean, complete := 0.0, true
		for i := end - window + 1; i <= end; i++ {
			value, valid := input.At(i)
			if !valid {
				complete = false
				break
			}
			mean += value / float64(window)
		}
		if !complete {
			continue
		}
		stddev, _ := stddevs.At(end)
		if stddev <= 1e-12 {
			continue
		}
		output.values[end] = (input.values[end] - mean) / stddev
		output.valid[end] = true
	}
	return output
}
