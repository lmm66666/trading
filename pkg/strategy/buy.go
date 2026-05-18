package strategy

import (
	"trading/pkg/filter"
)

func NewDailyB1BuyStrategy() *Strategy {
	cfg := filter.DefaultVolumeSurgeConfig()
	cfg.MinVolumeRatio = 2.0   // 倍量
	cfg.MinRallyPct = 5.0      // 大阳线
	cfg.MaxPullbackPct = 15.0  // 收紧回调幅度
	cfg.MaxPullbackDays = 10
	return NewStrategy("daily_b1_buy").
		AddFilter(filter.NewVolumeSurgeFilter(cfg)).
		AddFilter(filter.NewKDJOverSold(40)). // 放宽超卖条件
		AddFilter(filter.NewMATrendUp(20, 10))
}

// NewWeeklyB1BuyStrategy 周线 B1 买点策略
// 条件：
//  1. 周线 KDJ < 10（超卖）
//  2. 周线 MA20 在 MA60 之上（多头排列）
//  3. 收盘价站稳周 60 日均线
func NewWeeklyB1BuyStrategy() *Strategy {
	return NewStrategy("weekly_b1_buy").
		AddFilter(filter.NewKDJOverSold(10)).
		AddFilter(filter.NewMACrossFilter(20, 60)).
		AddFilter(filter.NewSupportHoldFilter(60))
}

// NewBottomSurgePullbackStrategy 底部放量拉升 + KDJ 低位回调策略
// 条件：
//  1. 放量日 Open 处于 60 日低点上浮 15% 以内（底部确认，在拉升起点检查）
//  2. 单日倍量大阳线（量比>=2.0, 涨幅>=5%）或渐进放量（连续3天量比>=1.2, 涨幅>=2%）
//  3. 允许拉升期间间隔 3 天内出现多次放量（主力买一天歇一天）
//  4. 回调幅度 <= 20%，天数 <= 15
//  5. MA20 在 MA60 之上
//  6. 收盘价在 MA60 之上
//  7. KDJ 的 J 值处于 -20~20 区间（超卖区，适合介入）
func NewBottomSurgePullbackStrategy() *Strategy {
	cfg := filter.VolumeSurgeConfig{
		VolumeMAPeriod:     20,
		MinVolumeRatio:     2.0,  // 单日倍量
		MinRallyPct:        5.0,  // 单日大阳线
		MaxPullbackPct:     20.0, // 回调幅度
		MaxPullbackDays:    15,   // 回调天数
		NearLowPeriod:      60,   // 底部确认
		NearLowMaxRatio:    0.15,
		SurgeWindowDays:    3,    // 允许两个放量日间隔3天
		SurgeMinDailyRatio: 1.2,  // 渐进放量单日量比
		SurgeMinDailyRally: 2.0,  // 渐进放量单日涨幅%
		SurgeMinConsecDays: 3,    // 渐进放量最低连续天数
	}
	return NewStrategy("bottom_surge_pullback").
		AddFilter(filter.NewVolumeSurgeFilter(cfg)).
		AddFilter(filter.NewMACrossFilter(20, 60)).
		AddFilter(filter.NewSupportHoldFilter(60)).
		AddFilter(filter.NewKDJRangeFilter(-20, 20))
}
