// Package indicator calculates deterministic, validity-aware market features.
package indicator

// Series is an immutable sequence where each value carries its own validity.
type Series struct {
	values []float64
	valid  []bool
}

func newSeries(values []float64, valid []bool) Series {
	return Series{values: values, valid: valid}
}

func invalidSeries(length int) Series {
	return Series{values: make([]float64, length), valid: make([]bool, length)}
}

// At returns false when index is out of bounds or the value has not warmed up.
func (s Series) At(index int) (float64, bool) {
	if index < 0 || index >= len(s.values) || index >= len(s.valid) || !s.valid[index] {
		return 0, false
	}
	return s.values[index], true
}

func (s Series) Valid(index int) bool {
	_, valid := s.At(index)
	return valid
}

func (s Series) Len() int {
	return len(s.values)
}

// Slice returns an independent half-open subseries. Invalid ranges return empty.
func (s Series) Slice(start, end int) Series {
	if start < 0 || end < start || end > len(s.values) || len(s.values) != len(s.valid) {
		return Series{}
	}
	values := append([]float64(nil), s.values[start:end]...)
	valid := append([]bool(nil), s.valid[start:end]...)
	return newSeries(values, valid)
}
