package filter

import (
	"trading/model"
	"trading/pkg/indicator"
)

type VolumeSurgeConfig struct {
	VolumeMAPeriod        int
	MinVolumeRatio        float64
	MinRallyPct           float64
	MaxPullbackPct        float64
	MaxPullbackDays       int
	MaxPullbackVolRatio   float64 // 回调期最大成交量比例（相对拉升期均量），0 表示不检查
	NearLowPeriod         int     // 底部确认周期（0=不检查），如 60
	NearLowMaxRatio       float64 // 底部确认最大上浮比例（0=不检查），如 0.15
	SurgeWindowDays       int     // 拉升窗口：允许两个放量日之间间隔的最大天数（0=当前行为，即不合并）
	MaxPullbackToVMARatio float64 // 回调均量相对 peak 处 VMA20 的最大比例（0=不检查）
	SurgeMinDailyRatio    float64 // 渐进放量单日最低量比（0=不检查），如 1.2
	SurgeMinDailyRally    float64 // 渐进放量单日最低涨幅%（0=不检查），如 2.0
	SurgeMinConsecDays    int     // 渐进放量最低连续天数（0=不检查），如 3
}

func DefaultVolumeSurgeConfig() VolumeSurgeConfig {
	return VolumeSurgeConfig{
		VolumeMAPeriod:      20,
		MinVolumeRatio:      1.2,
		MinRallyPct:         2.0,
		MaxPullbackPct:      20.0,
		MaxPullbackDays:     10,
		MaxPullbackVolRatio: 0,
	}
}

type pullbackWindow struct {
	surgeIdx  int
	peakIdx   int
	peakPrice float64
}

type VolumeSurgeFilter struct {
	Config VolumeSurgeConfig
}

func NewVolumeSurgeFilter(cfg VolumeSurgeConfig) *VolumeSurgeFilter {
	return &VolumeSurgeFilter{Config: cfg}
}

func (v *VolumeSurgeFilter) Filter(klines []*model.StockKline) []Result {
	cfg := v.Config
	n := len(klines)
	if n < cfg.VolumeMAPeriod+1 {
		return nil
	}

	volumes := make([]int64, n)
	for i, k := range klines {
		volumes[i] = k.Volume
	}

	vma := indicator.ComputeMA(volumes, cfg.VolumeMAPeriod)
	windows := findPullbackWindows(klines, volumes, vma, cfg)

	windowByDay := make(map[int]*pullbackWindow)
	for i := range windows {
		w := &windows[i]

		// NearLow 底部确认：检查窗口起点（第一个放量日）的 Open 是否在底部
		if cfg.NearLowPeriod > 0 && cfg.NearLowMaxRatio > 0 {
			start := 0
			if w.surgeIdx >= cfg.NearLowPeriod {
				start = w.surgeIdx - cfg.NearLowPeriod + 1
			}
			lowMin := klines[start].Low
			for j := start + 1; j <= w.surgeIdx; j++ {
				if klines[j].Low < lowMin {
					lowMin = klines[j].Low
				}
			}
			maxAllowed := lowMin * (1 + cfg.NearLowMaxRatio)
			if klines[w.surgeIdx].Open > maxAllowed {
				continue
			}
		}

		// 计算拉升期均量（用于缩量检查）
		var surgeAvgVol float64
		if cfg.MaxPullbackVolRatio > 0 {
			var surgeVolSum int64
			surgeDays := 0
			for d := w.surgeIdx; d <= w.peakIdx && d < n; d++ {
				surgeVolSum += volumes[d]
				surgeDays++
			}
			if surgeDays > 0 {
				surgeAvgVol = float64(surgeVolSum) / float64(surgeDays)
			}
		}

		for d := w.peakIdx + 1; d < n; d++ {
			pullbackPct := (w.peakPrice - klines[d].Close) / w.peakPrice * 100
			days := d - w.peakIdx
			if pullbackPct > cfg.MaxPullbackPct || days > cfg.MaxPullbackDays {
				break
			}

			// 缩量检查：回调期均量 <= 拉升期均量 * MaxPullbackVolRatio
			if cfg.MaxPullbackVolRatio > 0 && surgeAvgVol > 0 {
				var pullbackVolSum int64
				for pd := w.peakIdx + 1; pd <= d; pd++ {
					pullbackVolSum += volumes[pd]
				}
				pullbackAvgVol := float64(pullbackVolSum) / float64(days)
				if pullbackAvgVol > surgeAvgVol*cfg.MaxPullbackVolRatio {
					continue
				}
			}

			// 缩量检查（VMA20 基准）：回调期均量 <= peak 处 VMA20 * MaxPullbackToVMARatio
			if cfg.MaxPullbackToVMARatio > 0 && vma[w.peakIdx] > 0 {
				var pullbackVolSum int64
				for pd := w.peakIdx + 1; pd <= d; pd++ {
					pullbackVolSum += volumes[pd]
				}
				pullbackAvgVol := float64(pullbackVolSum) / float64(days)
				if pullbackAvgVol > vma[w.peakIdx]*cfg.MaxPullbackToVMARatio {
					continue
				}
			}

			if existing, ok := windowByDay[d]; !ok || w.peakPrice > existing.peakPrice {
				windowByDay[d] = w
			}
		}
	}

	results := make([]Result, n)
	for i := range n {
		_, ok := windowByDay[i]
		results[i] = Result{
			Date:  klines[i].Date,
			Valid: ok,
		}
	}
	return results
}

func findPullbackWindows(
	klines []*model.StockKline,
	volumes []int64,
	vma []float64,
	cfg VolumeSurgeConfig,
) []pullbackWindow {
	n := len(klines)
	var windows []pullbackWindow

	// 模式1：单日倍量检测
	for i := cfg.VolumeMAPeriod; i < n; i++ {
		if vma[i] == 0 {
			continue
		}
		volRatio := float64(volumes[i]) / vma[i]
		rallyPct := rallyFromPrevClose(klines, i)
		if volRatio < cfg.MinVolumeRatio || rallyPct < cfg.MinRallyPct {
			continue
		}

		lastSurgeDay := i
		if cfg.SurgeWindowDays > 0 {
			for j := i + 1; j < n && (j-lastSurgeDay) <= cfg.SurgeWindowDays; j++ {
				if vma[j] == 0 {
					continue
				}
				vr := float64(volumes[j]) / vma[j]
				rp := rallyFromPrevClose(klines, j)
				if vr >= cfg.MinVolumeRatio && rp >= cfg.MinRallyPct {
					lastSurgeDay = j
				}
			}
		}

		peakIdx, peakPrice := findPeak(klines, lastSurgeDay, n)

		windows = append(windows, pullbackWindow{
			surgeIdx:  i,
			peakIdx:   peakIdx,
			peakPrice: peakPrice,
		})

		if peakIdx > i {
			i = peakIdx
		}
	}

	// 模式2：渐进放量检测
	if cfg.SurgeMinConsecDays > 0 && cfg.SurgeMinDailyRatio > 0 && cfg.SurgeMinDailyRally > 0 {
		windows = append(windows, findGradualSurgeWindows(klines, volumes, vma, cfg)...)
	}

	return windows
}

// findGradualSurgeWindows 检测连续多日渐进放量上涨的窗口
func findGradualSurgeWindows(
	klines []*model.StockKline,
	volumes []int64,
	vma []float64,
	cfg VolumeSurgeConfig,
) []pullbackWindow {
	n := len(klines)
	var windows []pullbackWindow

	// 标记每天是否满足渐进放量条件
	meetsDaily := make([]bool, n)
	for i := cfg.VolumeMAPeriod; i < n; i++ {
		if vma[i] == 0 {
			continue
		}
		volRatio := float64(volumes[i]) / vma[i]
		rallyPct := rallyFromPrevClose(klines, i)
		meetsDaily[i] = volRatio >= cfg.SurgeMinDailyRatio && rallyPct >= cfg.SurgeMinDailyRally
	}

	// 寻找连续满足条件的区间
	start := -1
	for i := cfg.VolumeMAPeriod; i < n; i++ {
		if meetsDaily[i] {
			if start == -1 {
				start = i
			}
		} else {
			if start != -1 {
				end := i - 1
				consecDays := end - start + 1
				if consecDays >= cfg.SurgeMinConsecDays {
					if w := tryGradualWindow(klines, volumes, vma, cfg, start, end); w != nil {
						windows = append(windows, *w)
					}
				}
				start = -1
			}
		}
	}
	// 处理末尾
	if start != -1 {
		end := n - 1
		consecDays := end - start + 1
		if consecDays >= cfg.SurgeMinConsecDays {
			if w := tryGradualWindow(klines, volumes, vma, cfg, start, end); w != nil {
				windows = append(windows, *w)
			}
		}
	}

	return windows
}

// tryGradualWindow 验证渐进放量区间是否满足整体条件，满足则返回窗口
func tryGradualWindow(
	klines []*model.StockKline,
	volumes []int64,
	vma []float64,
	cfg VolumeSurgeConfig,
	start, end int,
) *pullbackWindow {
	n := len(klines)

	// 区间累计涨幅（从 start-1 的收盘到 end 的收盘）
	prevClose := klines[start].Open
	if start > 0 {
		prevClose = klines[start-1].Close
	}
	totalRally := (klines[end].Close - prevClose) / prevClose * 100
	if totalRally < cfg.MinRallyPct {
		return nil
	}

	// 区间均量比 >= MinVolumeRatio
	var volSum int64
	for d := start; d <= end; d++ {
		volSum += volumes[d]
	}
	days := end - start + 1
	avgVol := float64(volSum) / float64(days)
	var vmaSum float64
	for d := start; d <= end; d++ {
		vmaSum += vma[d]
	}
	avgVMA := vmaSum / float64(days)
	if avgVMA == 0 || avgVol/avgVMA < cfg.MinVolumeRatio {
		return nil
	}

	// SurgeWindowDays 扩展：允许区间末尾后有间隔的放量日
	lastSurgeDay := end
	if cfg.SurgeWindowDays > 0 {
		for j := end + 1; j < n && (j-lastSurgeDay) <= cfg.SurgeWindowDays; j++ {
			if meetsDaily(klines, volumes, vma, cfg, j) {
				lastSurgeDay = j
			}
		}
	}

	peakIdx, peakPrice := findPeak(klines, lastSurgeDay, n)

	return &pullbackWindow{
		surgeIdx:  start,
		peakIdx:   peakIdx,
		peakPrice: peakPrice,
	}
}

// meetsDaily 判断某天是否满足渐进放量的单日条件
func meetsDaily(klines []*model.StockKline, volumes []int64, vma []float64, cfg VolumeSurgeConfig, i int) bool {
	if vma[i] == 0 {
		return false
	}
	volRatio := float64(volumes[i]) / vma[i]
	rallyPct := rallyFromPrevClose(klines, i)
	return volRatio >= cfg.SurgeMinDailyRatio && rallyPct >= cfg.SurgeMinDailyRally
}

// findPeak 从 start 开始找连续收盘价上涨的峰值
func findPeak(klines []*model.StockKline, start, n int) (int, float64) {
	peakIdx := start
	peakPrice := klines[start].Close
	for j := start + 1; j < n; j++ {
		if klines[j].Close >= peakPrice {
			peakIdx = j
			peakPrice = klines[j].Close
		} else {
			break
		}
	}
	return peakIdx, peakPrice
}

// rallyFromPrevClose 计算相对前一日收盘价的涨幅百分比
func rallyFromPrevClose(klines []*model.StockKline, i int) float64 {
	if i == 0 {
		return (klines[i].Close - klines[i].Open) / klines[i].Open * 100
	}
	return (klines[i].Close - klines[i-1].Close) / klines[i-1].Close * 100
}
