package indicator

// EMA uses the conventional recursive definition: the first valid input seeds
// the sequence, then each next value uses alpha=2/(period+1). This makes the
// result prefix-invariant and avoids looking ahead for an initialization value.
func EMA(values []float64, period int) Series {
	input := newSeries(append([]float64(nil), values...), make([]bool, len(values)))
	for index := range input.valid {
		input.valid[index] = true
	}
	return emaSeries(input, period)
}

func emaSeries(input Series, period int) Series {
	result := invalidSeries(input.Len())
	if period <= 0 {
		return result
	}
	alpha := 2.0 / float64(period+1)
	previous := 0.0
	hasPrevious := false
	for index := 0; index < input.Len(); index++ {
		value, valid := input.At(index)
		if !valid {
			hasPrevious = false
			continue
		}
		if !hasPrevious {
			previous = value
			hasPrevious = true
		} else {
			previous = alpha*value + (1-alpha)*previous
		}
		result.values[index] = previous
		result.valid[index] = true
	}
	return result
}
