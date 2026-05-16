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
// 条件：周线 KDJ < 10（超卖）且周线 MA20 趋势向上
func NewWeeklyB1BuyStrategy() *Strategy {
	return NewStrategy("weekly_b1_buy").
		AddFilter(filter.NewKDJOverSold(10)).
		AddFilter(filter.NewMATrendUp(20, 1))
}

// NewBottomSurgePullbackStrategy 底部倍量拉升 + 缩量回调策略
// 条件：
//  1. 价格处于 60 日低点上浮 15% 以内（底部确认）
//  2. 出现倍量大阳线（成交量 >= MA20 * 2.0，涨幅 >= 5%）
//  3. 缩量回调（回调期均量 <= 拉升期均量 * 50%）
//  4. 回调幅度 <= 15%，天数 <= 10
//  5. 不破 MA20 支撑
//  6. KDJ 处于低位区间（5~40），适合介入
func NewBottomSurgePullbackStrategy() *Strategy {
	cfg := filter.VolumeSurgeConfig{
		VolumeMAPeriod:      20,
		MinVolumeRatio:      2.0,  // 倍量
		MinRallyPct:         5.0,  // 大阳线
		MaxPullbackPct:      15.0, // 浅幅回调
		MaxPullbackDays:     10,
		MaxPullbackVolRatio: 0.5, // 缩量 50%
	}
	return NewStrategy("bottom_surge_pullback").
		AddFilter(filter.NewNearLowFilter(60, 0.15)).       // 底部确认
		AddFilter(filter.NewVolumeSurgeFilter(cfg)).         // 倍量拉升 + 缩量回调
		AddFilter(filter.NewSupportHoldFilter(20)).          // 支撑有效
		AddFilter(filter.NewKDJRangeFilter(5, 40))           // KDJ 低位
}
