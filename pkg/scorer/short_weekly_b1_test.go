package scorer

import (
	"fmt"
	"testing"

	"trading/model"
)

func buildWeeklyB1Klines() ([]*model.StockKline, []*model.StockKline) {
	var weekly []*model.StockKline
	for i := 0; i < 80; i++ {
		closeP := 15.0 + float64(i)*0.02
		weekly = append(weekly, makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), closeP-0.3, closeP+0.3, closeP-0.5, closeP, 50000))
	}

	var daily []*model.StockKline
	for i := 0; i < 60; i++ {
		closeP := 15.5 + float64(i)*0.01
		daily = append(daily, makeKline("600000", fmt.Sprintf("2026-01-%02d", i+1), closeP-0.1, closeP+0.1, closeP-0.2, closeP, 10000))
	}

	return weekly, daily
}

func TestScoreWeeklyB1Short_InsufficientWeekly(t *testing.T) {
	weekly := []*model.StockKline{
		makeKline("600000", "2026-01-01", 10, 10.5, 9.5, 10, 10000),
		makeKline("600000", "2026-01-08", 10, 10.5, 9.5, 10, 10000),
	}
	daily := makeDailyKlines(60, 10.0, 10000)
	result := ScoreWeeklyB1Short(weekly, daily)

	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items should be empty for insufficient weekly data, got %d items", len(result.Items))
	}
}

func TestScoreWeeklyB1Short_WithValidData(t *testing.T) {
	weekly, daily := buildWeeklyB1Klines()
	result := ScoreWeeklyB1Short(weekly, daily)

	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	if len(result.Items) == 0 {
		t.Fatal("Items should not be empty for valid weekly B1 data")
	}

	maxSum := 0
	for _, item := range result.Items {
		maxSum += item.MaxScore
		if item.Score < 0 || item.Score > item.MaxScore {
			t.Errorf("item %q: Score=%d out of range [0, %d]", item.Name, item.Score, item.MaxScore)
		}
	}
	if maxSum != 100 {
		t.Errorf("MaxScore sum = %d, want 100", maxSum)
	}
}

func TestScoreWeeklyB1Short_NoDailyData(t *testing.T) {
	weekly, _ := buildWeeklyB1Klines()
	result := ScoreWeeklyB1Short(weekly, nil)

	if result.Max != 100 {
		t.Errorf("Max = %d, want 100", result.Max)
	}
	for _, item := range result.Items {
		if item.Name == "日线MA20支撑" && item.Score != 0 {
			t.Errorf("日线MA20支撑 should be 0 without daily data, got %d", item.Score)
		}
	}
}

func TestInverseRangeScore(t *testing.T) {
	tests := []struct {
		name            string
		val, t1, t2, t3 float64
		s1, s2, s3      int
		want            int
	}{
		{"below_t1", -5, 0, 10, 20, 20, 15, 10, 20},
		{"equal_t1", 0, 0, 10, 20, 20, 15, 10, 15},
		{"between_t1_t2", 5, 0, 10, 20, 20, 15, 10, 15},
		{"between_t2_t3", 15, 0, 10, 20, 20, 15, 10, 10},
		{"above_t3", 25, 0, 10, 20, 20, 15, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inverseRangeScore(tt.val, tt.t1, tt.t2, tt.t3, tt.s1, tt.s2, tt.s3)
			if got != tt.want {
				t.Errorf("inverseRangeScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestKdSyncScore(t *testing.T) {
	tests := []struct {
		name string
		k, d float64
		want int
	}{
		{"both_below_20", 15, 18, 10},
		{"k_below_20_d_above", 15, 25, 5},
		{"both_above_30", 35, 40, 0},
		{"k_25_d_25", 25, 25, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kdSyncScore(tt.k, tt.d)
			if got != tt.want {
				t.Errorf("kdSyncScore(%v, %v) = %d, want %d", tt.k, tt.d, got, tt.want)
			}
		})
	}
}

func TestCloseMAScore(t *testing.T) {
	tests := []struct {
		name      string
		close, ma float64
		want      int
	}{
		{"above_ma", 11, 10, 10},
		{"near_ma", 10.1, 10, 10},
		{"touch_ma", 9.85, 10, 5},
		{"below_ma", 9.5, 10, 0},
		{"zero_ma", 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := closeMAScore(tt.close, tt.ma)
			if got != tt.want {
				t.Errorf("closeMAScore(%v, %v) = %d, want %d", tt.close, tt.ma, got, tt.want)
			}
		})
	}
}

func TestAdjustWeeksScore(t *testing.T) {
	tests := []struct {
		name  string
		weeks int
		want  int
	}{
		{"short", 5, 5},
		{"at_8", 8, 5},
		{"medium", 12, 3},
		{"at_16", 16, 3},
		{"long", 20, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustWeeksScore(tt.weeks)
			if got != tt.want {
				t.Errorf("adjustWeeksScore(%d) = %d, want %d", tt.weeks, got, tt.want)
			}
		})
	}
}
