package indicator

import "trading/model"

type KDJResult struct {
	Date string  `json:"date"`
	K    float64 `json:"k"`
	D    float64 `json:"d"`
	J    float64 `json:"j"`
}

// slidingWindowMax 双端队列求滑动窗口最大值（窗口大小动态：min(i+1, period)）
func slidingWindowMax(arr []float64, period int) []float64 {
	n := len(arr)
	result := make([]float64, n)
	deque := make([]int, 0, period)

	for i := range arr {
		for len(deque) > 0 && deque[0] < i-period+1 {
			deque = deque[1:]
		}
		for len(deque) > 0 && arr[deque[len(deque)-1]] <= arr[i] {
			deque = deque[:len(deque)-1]
		}
		deque = append(deque, i)
		result[i] = arr[deque[0]]
	}
	return result
}

// slidingWindowMin 双端队列求滑动窗口最小值（窗口大小动态：min(i+1, period)）
func slidingWindowMin(arr []float64, period int) []float64 {
	n := len(arr)
	result := make([]float64, n)
	deque := make([]int, 0, period)

	for i := range arr {
		for len(deque) > 0 && deque[0] < i-period+1 {
			deque = deque[1:]
		}
		for len(deque) > 0 && arr[deque[len(deque)-1]] >= arr[i] {
			deque = deque[:len(deque)-1]
		}
		deque = append(deque, i)
		result[i] = arr[deque[0]]
	}
	return result
}

// ComputeKDJ 标准 KDJ: RSV(9), K/D 初始 50，滑动窗口 O(n)
func ComputeKDJ(klines []*model.StockKline) []KDJResult {
	n := len(klines)
	if n == 0 {
		return nil
	}

	const period = 9

	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i, k := range klines {
		highs[i] = k.High
		lows[i] = k.Low
		closes[i] = k.Close
	}

	highMaxes := slidingWindowMax(highs, period)
	lowMins := slidingWindowMin(lows, period)

	results := make([]KDJResult, n)
	kVal, dVal := 50.0, 50.0

	for i := range klines {
		highMax := highMaxes[i]
		lowMin := lowMins[i]

		rsv := 50.0
		if highMax != lowMin {
			rsv = (closes[i] - lowMin) / (highMax - lowMin) * 100
		}

		kVal = 2.0/3*kVal + 1.0/3*rsv
		dVal = 2.0/3*dVal + 1.0/3*kVal
		jVal := 3*kVal - 2*dVal

		results[i] = KDJResult{
			Date: klines[i].Date,
			K:    Round4(kVal),
			D:    Round4(dVal),
			J:    Round4(jVal),
		}
	}
	return results
}
