package filter

import (
	"fmt"
	"testing"

	"trading/pkg/indicator"
)

func TestDebug600150VolumeSurgeDetail(t *testing.T) {
	klines := klines600150
	n := len(klines)

	volumes := make([]int64, n)
	for i, k := range klines {
		volumes[i] = k.Volume
	}
	vma := indicator.ComputeMA(volumes, 20)

	cfg := VolumeSurgeConfig{
		VolumeMAPeriod:        20,
		MinVolumeRatio:        2.0,
		MinRallyPct:           5.0,
		MaxPullbackPct:        15.0,
		MaxPullbackDays:       10,
		MaxPullbackVolRatio:   0,
		NearLowPeriod:         60,
		NearLowMaxRatio:       0.15,
		SurgeWindowDays:       3,
		MaxPullbackToVMARatio: 1.5,
	}

	windows := findPullbackWindows(klines, volumes, vma, cfg.VolumeMAPeriod, cfg.MinVolumeRatio, cfg.MinRallyPct, cfg.SurgeWindowDays)
	fmt.Printf("=== 发现的放量窗口（SurgeWindowDays=3）===\n")
	fmt.Printf("共 %d 个窗口\n\n", len(windows))

	for i, w := range windows {
		surgeK := klines[w.surgeIdx]
		peakK := klines[w.peakIdx]

		fmt.Printf("窗口%d: surgeStart=%s(O=%.2f) peak=%s(C=%.2f)\n",
			i, surgeK.Date, surgeK.Open, peakK.Date, peakK.Close)

		for d := w.surgeIdx; d <= w.peakIdx && d < n; d++ {
			k := klines[d]
			volRatio := 0.0
			if vma[d] > 0 {
				volRatio = float64(k.Volume) / vma[d]
			}
			rallyPct := (k.Close - k.Open) / k.Open * 100
			fmt.Printf("  拉升 %s O=%.2f C=%.2f V=%d 量比=%.2f 涨幅=%.2f%%\n",
				k.Date, k.Open, k.Close, k.Volume, volRatio, rallyPct)
		}

		for d := w.peakIdx + 1; d < n; d++ {
			pullbackPct := (w.peakPrice - klines[d].Close) / w.peakPrice * 100
			days := d - w.peakIdx
			if pullbackPct > cfg.MaxPullbackPct || days > cfg.MaxPullbackDays {
				fmt.Printf("  -> %s 回调=%.2f%% 天数=%d 超出范围\n", klines[d].Date, pullbackPct, days)
				break
			}
			var pullbackVolSum int64
			for pd := w.peakIdx + 1; pd <= d; pd++ {
				pullbackVolSum += volumes[pd]
			}
			pullbackAvgVol := float64(pullbackVolSum) / float64(days)
			vmaRatio := 0.0
			if vma[w.peakIdx] > 0 {
				vmaRatio = pullbackAvgVol / vma[w.peakIdx]
			}
			fmt.Printf("  回调 %s C=%.2f 回调=%.2f%% 天数=%d 均量=%.0f VMA比=%.2f %s\n",
				klines[d].Date, klines[d].Close, pullbackPct, days,
				pullbackAvgVol, vmaRatio,
				map[bool]string{true: "PASS", false: "FAIL"}[vmaRatio <= cfg.MaxPullbackToVMARatio])
		}
		fmt.Println()
	}
}
