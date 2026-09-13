package indicator

// SMA returns a full-window simple moving average. Values before period are invalid.
func SMA(values []float64, period int) Series {
	series := newSeries(append([]float64(nil), values...), make([]bool, len(values)))
	for index := range series.valid {
		series.valid[index] = true
	}
	if period <= 0 {
		return invalidSeries(len(values))
	}
	return smaSeries(series, period)
}

func smaSeries(input Series, period int) Series {
	result := invalidSeries(input.Len())
	if period <= 0 {
		return result
	}
	var sum float64
	invalid := 0
	for index := 0; index < input.Len(); index++ {
		value, valid := input.At(index)
		if valid {
			sum += value
		} else {
			invalid++
		}
		if index >= period {
			previous, previousValid := input.At(index - period)
			if previousValid {
				sum -= previous
			} else {
				invalid--
			}
		}
		if index >= period-1 && invalid == 0 {
			result.values[index] = sum / float64(period)
			result.valid[index] = true
		}
	}
	return result
}
