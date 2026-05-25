package business

import (
	"fmt"
	"strings"
	"time"

	"trading/model"
	"trading/pkg/indicator"
)

// toSymbol 将纯数字 code 转换为带前缀的 symbol
func toSymbol(code string) (string, error) {
	switch {
	case strings.HasPrefix(code, "6"):
		return "sh" + code, nil
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "3"):
		return "sz" + code, nil
	default:
		return "", fmt.Errorf("unsupported stock code: %s", code)
	}
}

// cleanKlines 去掉 Code 中的 sh/sz 前缀
func cleanKlines(klines []model.StockKline) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(klines))
	for i := range klines {
		k := &klines[i]
		k.Code = strings.TrimPrefix(k.Code, "sh")
		k.Code = strings.TrimPrefix(k.Code, "sz")
		result = append(result, k)
	}
	return result
}

// filterAfterDate 过滤出日期严格大于 lastDate 的 K 线
func filterAfterDate(klines []*model.StockKline, lastDate string) []*model.StockKline {
	if lastDate == "" {
		return klines
	}
	result := make([]*model.StockKline, 0, len(klines))
	for _, k := range klines {
		if k.Date > lastDate {
			result = append(result, k)
		}
	}
	return result
}

// filterIncompleteWeekly 过滤掉最后一条非周五的周线数据
// broker 返回的周线若最后一条是周中日期，则为不完整周，应丢弃
func filterIncompleteWeekly(klines []*model.StockKline) []*model.StockKline {
	if len(klines) == 0 {
		return klines
	}

	last := klines[len(klines)-1]
	if !isFriday(last.Date) {
		return klines[:len(klines)-1]
	}
	return klines
}

// isFriday 判断日期字符串是否为周五
// 支持格式：2006-01-02 或 2006-01-02 15:04:05
func isFriday(dateStr string) bool {
	layout := "2006-01-02"
	if len(dateStr) > 10 {
		layout = "2006-01-02 15:04:05"
	}
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return false
	}
	return t.Weekday() == time.Friday
}

// toDaily 将通用 K 线转换为日线模型
func toDaily(klines []*model.StockKline) []*model.StockKlineDaily {
	result := make([]*model.StockKlineDaily, 0, len(klines))
	for _, k := range klines {
		d := model.StockKlineDaily(*k)
		result = append(result, &d)
	}
	return result
}

// toWeekly 将通用 K 线转换为周线模型
func toWeekly(klines []*model.StockKline) []*model.StockKlineWeekly {
	result := make([]*model.StockKlineWeekly, 0, len(klines))
	for _, k := range klines {
		w := model.StockKlineWeekly(*k)
		result = append(result, &w)
	}
	return result
}

// dailyToKlines 将日线模型转换为通用 K 线
func dailyToKlines(dailies []*model.StockKlineDaily) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(dailies))
	for _, d := range dailies {
		k := model.StockKline(*d)
		result = append(result, &k)
	}
	return result
}

// weeklyToKlines 将周线模型转换为通用 K 线
func weeklyToKlines(weeklies []*model.StockKlineWeekly) []*model.StockKline {
	result := make([]*model.StockKline, 0, len(weeklies))
	for _, w := range weeklies {
		k := model.StockKline(*w)
		result = append(result, &k)
	}
	return result
}

// computeDailyMA20Map 计算日线 20 日均线并返回日期 -> 均线值映射
func computeDailyMA20Map(dailies []*model.StockKlineDaily) map[string]float64 {
	n := len(dailies)
	if n == 0 {
		return nil
	}

	prices := make([]float64, n)
	for i, d := range dailies {
		prices[i] = d.Close
	}

	maResults := indicator.ComputeMA(prices, 20)
	result := make(map[string]float64, n)
	for i := range n {
		result[dailies[i].Date] = maResults[i]
	}
	return result
}

// lastFridayDate 返回最近一个周五的日期字符串
func lastFridayDate(t time.Time) string {
	wd := t.Weekday()
	daysBack := int(wd - time.Friday)
	if daysBack < 0 {
		daysBack += 7
	}
	if wd == time.Friday {
		daysBack = 0
	}
	return t.Add(-time.Duration(daysBack) * 24 * time.Hour).Format("2006-01-02")
}

// isWeekday 判断是否为工作日（周一到周五）
func isWeekday(t time.Time) bool {
	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}
