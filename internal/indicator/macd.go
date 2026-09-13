package indicator

func macd(input Series, fast, slow, signal int) (Series, Series, Series) {
	fastEMA := emaSeries(input, fast)
	slowEMA := emaSeries(input, slow)
	dif := invalidSeries(input.Len())
	for index := 0; index < input.Len(); index++ {
		fastValue, fastValid := fastEMA.At(index)
		slowValue, slowValid := slowEMA.At(index)
		if fastValid && slowValid {
			dif.values[index] = fastValue - slowValue
			dif.valid[index] = true
		}
	}
	dea := emaSeries(dif, signal)
	histogram := invalidSeries(input.Len())
	for index := 0; index < input.Len(); index++ {
		difValue, difValid := dif.At(index)
		deaValue, deaValid := dea.At(index)
		if difValid && deaValid {
			histogram.values[index] = 2 * (difValue - deaValue)
			histogram.valid[index] = true
		}
	}
	return dif, dea, histogram
}
