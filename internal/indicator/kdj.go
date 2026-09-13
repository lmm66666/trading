package indicator

func kdj(high, low, close Series, period int) (Series, Series, Series) {
	k := invalidSeries(close.Len())
	d := invalidSeries(close.Len())
	j := invalidSeries(close.Len())
	if period <= 0 || high.Len() != close.Len() || low.Len() != close.Len() {
		return k, d, j
	}
	kValue, dValue := 50.0, 50.0
	for index := 0; index < close.Len(); index++ {
		if index < period-1 {
			continue
		}
		lowest, highest, valid := kdjRange(high, low, index-period+1, index+1)
		closeValue, closeValid := close.At(index)
		if !valid || !closeValid {
			continue
		}
		rsv := 50.0
		if highest != lowest {
			rsv = (closeValue - lowest) / (highest - lowest) * 100
		}
		kValue = 2.0/3*kValue + 1.0/3*rsv
		dValue = 2.0/3*dValue + 1.0/3*kValue
		k.values[index], k.valid[index] = kValue, true
		d.values[index], d.valid[index] = dValue, true
		j.values[index], j.valid[index] = 3*kValue-2*dValue, true
	}
	return k, d, j
}

func kdjRange(high, low Series, start, end int) (float64, float64, bool) {
	var lowest, highest float64
	for index := start; index < end; index++ {
		highValue, highValid := high.At(index)
		lowValue, lowValid := low.At(index)
		if !highValid || !lowValid {
			return 0, 0, false
		}
		if index == start || lowValue < lowest {
			lowest = lowValue
		}
		if index == start || highValue > highest {
			highest = highValue
		}
	}
	return lowest, highest, true
}
