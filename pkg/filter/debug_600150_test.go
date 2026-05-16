package filter

import (
	"fmt"
	"testing"

	"trading/model"
	"trading/pkg/indicator"
)

// 600150 中国船舶 70 天日线数据（2026-01-26 ~ 2026-05-15）
var klines600150 = []*model.StockKline{
	{Code: "600150", Date: "2026-01-26", Open: 35.56, High: 36.1, Low: 35.14, Close: 35.34, Volume: 98119066},
	{Code: "600150", Date: "2026-01-27", Open: 35.33, High: 35.35, Low: 34.77, Close: 34.9, Volume: 68569549},
	{Code: "600150", Date: "2026-01-28", Open: 35.03, High: 35.75, Low: 34.62, Close: 34.69, Volume: 95337191},
	{Code: "600150", Date: "2026-01-29", Open: 34.62, High: 34.73, Low: 33.91, Close: 34.57, Volume: 99660167},
	{Code: "600150", Date: "2026-01-30", Open: 34.6, High: 34.71, Low: 33.21, Close: 33.54, Volume: 105907259},
	{Code: "600150", Date: "2026-02-02", Open: 33.7, High: 34.46, Low: 33.05, Close: 33.1, Volume: 100722489},
	{Code: "600150", Date: "2026-02-03", Open: 33.4, High: 34.71, Low: 33.26, Close: 34.62, Volume: 135565642},
	{Code: "600150", Date: "2026-02-04", Open: 34.49, High: 35.6, Low: 34.39, Close: 35.11, Volume: 110459038},
	{Code: "600150", Date: "2026-02-05", Open: 34.99, High: 35.24, Low: 34.38, Close: 34.66, Volume: 56507585},
	{Code: "600150", Date: "2026-02-06", Open: 34.42, High: 34.53, Low: 33.82, Close: 33.91, Volume: 64075204},
	{Code: "600150", Date: "2026-02-09", Open: 34.21, High: 34.36, Low: 33.93, Close: 34.09, Volume: 43096589},
	{Code: "600150", Date: "2026-02-10", Open: 34.12, High: 34.85, Low: 33.91, Close: 34.8, Volume: 87577336},
	{Code: "600150", Date: "2026-02-11", Open: 34.75, High: 35.44, Low: 34.56, Close: 34.99, Volume: 85718498},
	{Code: "600150", Date: "2026-02-12", Open: 34.85, High: 36.48, Low: 34.69, Close: 36.06, Volume: 145463583},
	{Code: "600150", Date: "2026-02-13", Open: 35.9, High: 36.75, Low: 35.87, Close: 36.35, Volume: 125975892},
	{Code: "600150", Date: "2026-02-24", Open: 36.69, High: 37.9, Low: 36.4, Close: 37.32, Volume: 135390764},
	{Code: "600150", Date: "2026-02-25", Open: 37.14, High: 38.3, Low: 37.12, Close: 38.1, Volume: 151281784},
	{Code: "600150", Date: "2026-02-26", Open: 37.95, High: 38.2, Low: 37.68, Close: 38.11, Volume: 86716008},
	{Code: "600150", Date: "2026-02-27", Open: 38, High: 38.28, Low: 37.59, Close: 37.75, Volume: 87861386},
	{Code: "600150", Date: "2026-03-02", Open: 38.1, High: 38.38, Low: 37.67, Close: 38.13, Volume: 118576237},
	{Code: "600150", Date: "2026-03-03", Open: 38.31, High: 39.07, Low: 37.48, Close: 37.57, Volume: 131806679},
	{Code: "600150", Date: "2026-03-04", Open: 37.29, High: 38.86, Low: 36.78, Close: 38.56, Volume: 150904754},
	{Code: "600150", Date: "2026-03-05", Open: 38.8, High: 39.27, Low: 38.51, Close: 38.76, Volume: 106074241},
	{Code: "600150", Date: "2026-03-06", Open: 38.55, High: 39.45, Low: 38.43, Close: 38.93, Volume: 98363388},
	{Code: "600150", Date: "2026-03-09", Open: 38.64, High: 38.78, Low: 37.25, Close: 37.32, Volume: 122358067},
	{Code: "600150", Date: "2026-03-10", Open: 37.6, High: 37.96, Low: 36.95, Close: 37, Volume: 80330788},
	{Code: "600150", Date: "2026-03-11", Open: 37.01, High: 37.09, Low: 36.37, Close: 36.56, Volume: 80461162},
	{Code: "600150", Date: "2026-03-12", Open: 36.58, High: 36.65, Low: 35.77, Close: 36, Volume: 78018865},
	{Code: "600150", Date: "2026-03-13", Open: 35.89, High: 35.98, Low: 35.08, Close: 35.18, Volume: 79038055},
	{Code: "600150", Date: "2026-03-16", Open: 35.6, High: 35.61, Low: 34.63, Close: 34.91, Volume: 81046867},
	{Code: "600150", Date: "2026-03-17", Open: 34.95, High: 35.2, Low: 34.36, Close: 34.4, Volume: 80126409},
	{Code: "600150", Date: "2026-03-18", Open: 34.46, High: 34.6, Low: 34.02, Close: 34.58, Volume: 70802135},
	{Code: "600150", Date: "2026-03-19", Open: 34.17, High: 34.71, Low: 34.05, Close: 34.16, Volume: 80195292},
	{Code: "600150", Date: "2026-03-20", Open: 34.38, High: 34.41, Low: 33.15, Close: 33.15, Volume: 97072399},
	{Code: "600150", Date: "2026-03-23", Open: 32.49, High: 32.5, Low: 30.83, Close: 31.13, Volume: 138416124},
	{Code: "600150", Date: "2026-03-24", Open: 31.52, High: 31.73, Low: 31.1, Close: 31.58, Volume: 77408427},
	{Code: "600150", Date: "2026-03-25", Open: 31.81, High: 32.06, Low: 31.64, Close: 31.85, Volume: 71482594},
	{Code: "600150", Date: "2026-03-26", Open: 31.85, High: 31.85, Low: 30.52, Close: 30.67, Volume: 83268838},
	{Code: "600150", Date: "2026-03-27", Open: 30.29, High: 30.98, Low: 30, Close: 30.84, Volume: 52924162},
	{Code: "600150", Date: "2026-03-30", Open: 30.35, High: 30.78, Low: 30.16, Close: 30.55, Volume: 56715398},
	{Code: "600150", Date: "2026-03-31", Open: 30.9, High: 31.53, Low: 30.81, Close: 30.84, Volume: 82588127},
	{Code: "600150", Date: "2026-04-01", Open: 31.3, High: 31.44, Low: 30.88, Close: 31.05, Volume: 57389820},
	{Code: "600150", Date: "2026-04-02", Open: 31, High: 31.21, Low: 30.62, Close: 30.9, Volume: 47942673},
	{Code: "600150", Date: "2026-04-03", Open: 30.91, High: 31, Low: 30.4, Close: 30.51, Volume: 36458329},
	{Code: "600150", Date: "2026-04-07", Open: 30.55, High: 32.29, Low: 30.55, Close: 31.98, Volume: 112541962},
	{Code: "600150", Date: "2026-04-08", Open: 32.26, High: 32.43, Low: 31.81, Close: 32.42, Volume: 101929841},
	{Code: "600150", Date: "2026-04-09", Open: 32.15, High: 32.33, Low: 31.91, Close: 32.23, Volume: 55274455},
	{Code: "600150", Date: "2026-04-10", Open: 32.3, High: 32.88, Low: 32.3, Close: 32.65, Volume: 60043928},
	{Code: "600150", Date: "2026-04-13", Open: 32.37, High: 32.83, Low: 32.23, Close: 32.66, Volume: 52105045},
	{Code: "600150", Date: "2026-04-14", Open: 33.28, High: 33.4, Low: 32.71, Close: 33, Volume: 58118095},
	{Code: "600150", Date: "2026-04-15", Open: 33.2, High: 33.2, Low: 32.55, Close: 32.71, Volume: 50890879},
	{Code: "600150", Date: "2026-04-16", Open: 32.76, High: 33.18, Low: 32.63, Close: 32.72, Volume: 54593888},
	{Code: "600150", Date: "2026-04-17", Open: 32.61, High: 33.12, Low: 32.24, Close: 33.03, Volume: 63535182},
	{Code: "600150", Date: "2026-04-20", Open: 33.12, High: 35.42, Low: 33.09, Close: 35.28, Volume: 179660138},
	{Code: "600150", Date: "2026-04-21", Open: 35.44, High: 36.61, Low: 35.44, Close: 36.32, Volume: 149408849},
	{Code: "600150", Date: "2026-04-22", Open: 36, High: 36.64, Low: 35.9, Close: 36.48, Volume: 90879954},
	{Code: "600150", Date: "2026-04-23", Open: 36.52, High: 38.91, Low: 36.51, Close: 38.6, Volume: 241955281},
	{Code: "600150", Date: "2026-04-24", Open: 38.6, High: 39.2, Low: 38.01, Close: 38.8, Volume: 160159919},
	{Code: "600150", Date: "2026-04-27", Open: 38.8, High: 39.3, Low: 38.08, Close: 38.41, Volume: 108525685},
	{Code: "600150", Date: "2026-04-28", Open: 38.71, High: 41.97, Low: 38.59, Close: 41.32, Volume: 262889560},
	{Code: "600150", Date: "2026-04-29", Open: 41, High: 41.48, Low: 40.84, Close: 41.16, Volume: 112260162},
	{Code: "600150", Date: "2026-04-30", Open: 43, High: 43.42, Low: 41.5, Close: 41.76, Volume: 253377515},
	{Code: "600150", Date: "2026-05-06", Open: 41.02, High: 41.52, Low: 39.74, Close: 41.18, Volume: 224558770},
	{Code: "600150", Date: "2026-05-07", Open: 40.99, High: 41.93, Low: 40.38, Close: 40.61, Volume: 125375689},
	{Code: "600150", Date: "2026-05-08", Open: 40.6, High: 41.8, Low: 39.93, Close: 41.15, Volume: 134081655},
	{Code: "600150", Date: "2026-05-11", Open: 40.89, High: 41.38, Low: 40.43, Close: 41.18, Volume: 114518475},
	{Code: "600150", Date: "2026-05-12", Open: 41.2, High: 41.68, Low: 40.09, Close: 40.5, Volume: 112820240},
	{Code: "600150", Date: "2026-05-13", Open: 40.6, High: 42.04, Low: 40.6, Close: 41.77, Volume: 148353985},
	{Code: "600150", Date: "2026-05-14", Open: 41.56, High: 41.8, Low: 40.2, Close: 40.22, Volume: 100748840},
	{Code: "600150", Date: "2026-05-15", Open: 40.8, High: 41.66, Low: 40.1, Close: 40.55, Volume: 100447757},
}

func TestDebug600150EachFilter(t *testing.T) {
	klines := klines600150
	n := len(klines)

	type namedFilter struct {
		name   string
		filter Filter
	}
	filters := []namedFilter{
		{"VolumeSurge(含底部确认+窗口合并)", NewVolumeSurgeFilter(VolumeSurgeConfig{
			VolumeMAPeriod: 20, MinVolumeRatio: 2.0, MinRallyPct: 5.0,
			MaxPullbackPct: 15.0, MaxPullbackDays: 10, MaxPullbackVolRatio: 0,
			NearLowPeriod: 60, NearLowMaxRatio: 0.15,
			SurgeWindowDays: 3, MaxPullbackToVMARatio: 1.5,
		})},
		{"SupportHold(MA20)", NewSupportHoldFilter(20)},
		{"KDJRange(5,40)", NewKDJRangeFilter(5, 40)},
	}

	for _, f := range filters {
		results := f.filter.Filter(klines)
		validDays := 0
		lastValid := ""
		for i, r := range results {
			if r.Valid {
				validDays++
				lastValid = klines[i].Date
			}
		}
		fmt.Printf("\n[%s] 有效天数: %d, 最后有效日: %s\n", f.name, validDays, lastValid)
		for i := n - 10; i < n; i++ {
			fmt.Printf("  %s Valid=%v\n", results[i].Date, results[i].Valid)
		}
	}

	// MA20 & KDJ
	prices := make([]float64, n)
	for i, k := range klines {
		prices[i] = k.Close
	}
	ma20 := indicator.ComputeMA(prices, 20)
	kdjResults := indicator.ComputeKDJ(klines)
	fmt.Printf("\n=== 最后15天 MA20 & KDJ ===\n")
	for i := n - 15; i < n; i++ {
		k := klines[i]
		fmt.Printf("%s C=%.2f MA20=%.2f K=%.2f D=%.2f J=%.2f\n",
			k.Date, k.Close, ma20[i], kdjResults[i].K, kdjResults[i].D, kdjResults[i].J)
	}
}
