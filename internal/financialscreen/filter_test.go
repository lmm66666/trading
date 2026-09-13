package financialscreen_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/financialscreen"
)

func TestGrowthFilterRejectsInvalidConfiguration(t *testing.T) {
	for _, threshold := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := financialscreen.NewProfitGrowth(threshold, 4)
		require.Error(t, err)
	}
	_, err := financialscreen.NewRevenueGrowth(0.1, 0)
	require.Error(t, err)
}
