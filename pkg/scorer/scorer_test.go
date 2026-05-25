package scorer

import (
	"fmt"
	"testing"

	"trading/model"
)

func TestStepScore(t *testing.T) {
	tests := []struct {
		name       string
		value      float64
		t1, t2, t3 float64
		want       int
	}{
		{"above_t1", 6.0, 5.0, 3.0, 2.0, 20},
		{"equal_t1", 5.0, 5.0, 3.0, 2.0, 20},
		{"between_t1_t2", 4.0, 5.0, 3.0, 2.0, 15},
		{"equal_t2", 3.0, 5.0, 3.0, 2.0, 15},
		{"between_t2_t3", 2.5, 5.0, 3.0, 2.0, 10},
		{"equal_t3", 2.0, 5.0, 3.0, 2.0, 10},
		{"below_t3", 1.0, 5.0, 3.0, 2.0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepScore(tt.value, tt.t1, tt.t2, tt.t3)
			if got != tt.want {
				t.Errorf("stepScore(%v) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestInverseStepScore(t *testing.T) {
	tests := []struct {
		name              string
		value             float64
		t1, t2, t3        float64
		s1, s2, s3        int
		want              int
	}{
		{"below_t1", 3.0, 5.0, 10.0, 15.0, 15, 10, 5, 15},
		{"equal_t1", 5.0, 5.0, 10.0, 15.0, 15, 10, 5, 15},
		{"between_t1_t2", 7.0, 5.0, 10.0, 15.0, 15, 10, 5, 10},
		{"equal_t2", 10.0, 5.0, 10.0, 15.0, 15, 10, 5, 10},
		{"between_t2_t3", 12.0, 5.0, 10.0, 15.0, 15, 10, 5, 5},
		{"equal_t3", 15.0, 5.0, 10.0, 15.0, 15, 10, 5, 5},
		{"above_t3", 20.0, 5.0, 10.0, 15.0, 15, 10, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inverseStepScore(tt.value, tt.t1, tt.t2, tt.t3, tt.s1, tt.s2, tt.s3)
			if got != tt.want {
				t.Errorf("inverseStepScore(%v) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestIntScore(t *testing.T) {
	tests := []struct {
		name      string
		v, h, m   int
		want      int
	}{
		{"above_h", 5, 3, 2, 10},
		{"equal_h", 3, 3, 2, 10},
		{"between_h_m", 2, 3, 2, 5},
		{"below_m", 1, 3, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intScore(tt.v, tt.h, tt.m)
			if got != tt.want {
				t.Errorf("intScore(%d, %d, %d) = %d, want %d", tt.v, tt.h, tt.m, got, tt.want)
			}
		})
	}
}

func TestRatioScore(t *testing.T) {
	tests := []struct {
		name        string
		v, h, m     float64
		high, mid   int
		want        int
	}{
		{"above_h", 0.8, 0.7, 0.5, 5, 3, 5},
		{"between_h_m", 0.6, 0.7, 0.5, 5, 3, 3},
		{"below_m", 0.3, 0.7, 0.5, 5, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ratioScore(tt.v, tt.h, tt.m, tt.high, tt.mid)
			if got != tt.want {
				t.Errorf("ratioScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScoreDetailTotal(t *testing.T) {
	d := &ScoreDetail{
		Max: 100,
		Items: []ScoreItem{
			{Name: "A", Score: 20, MaxScore: 20},
			{Name: "B", Score: 15, MaxScore: 20},
			{Name: "C", Score: 5, MaxScore: 10},
		},
		Total: 40,
	}
	if d.Total != 40 {
		t.Errorf("Total = %d, want 40", d.Total)
	}
}

// ── 辅助：构造 K 线 ──

func makeKline(code, date string, open, high, low, close float64, volume int64) *model.StockKline {
	return &model.StockKline{
		Code:   code,
		Date:   date,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}
}

func makeDailyKlines(n int, basePrice float64, baseVol int64) []*model.StockKline {
	klines := make([]*model.StockKline, n)
	for i := 0; i < n; i++ {
		date := fmt.Sprintf("2026-01-%02d", i+1)
		klines[i] = makeKline("600000", date, basePrice, basePrice+0.5, basePrice-0.5, basePrice, baseVol)
	}
	return klines
}
