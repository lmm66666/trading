package backtest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"trading/internal/market"
)

func TestFixedPointRoundingFloorsQuantitiesAndCeilsCosts(t *testing.T) {
	floor, floorOK := mulDivFloor(10, 1, 3)
	ceil, ceilOK := mulDivCeil(10, 1, 3)
	zero, zeroOK := mulDivCeil(0, 9, 7)
	_, invalid := mulDivFloor(1, 1, 0)
	_, overflow := mulDivFloor(math.MaxInt64, 2, 1)
	_, moneyOverflow := addMoney(market.Money(math.MaxInt64), 1)

	assert.True(t, floorOK)
	assert.EqualValues(t, 3, floor)
	assert.True(t, ceilOK)
	assert.EqualValues(t, 4, ceil)
	assert.True(t, zeroOK)
	assert.Zero(t, zero)
	assert.False(t, invalid)
	assert.False(t, overflow)
	assert.False(t, moneyOverflow)
}
