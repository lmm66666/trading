package market_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trading/internal/market"
)

func TestNewDatasetSortsRejectsDuplicatesAndInvalidOHLC(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bars := []market.Bar{
		testBar(id, "2026-01-06", 102000, 105000, 101000, 104000),
		testBar(id, "2026-01-05", 100000, 103000, 99000, 102000),
	}
	got, err := market.NewDataset(id, market.Day, 7, bars)
	require.NoError(t, err)
	assert.True(t, got.Bar(0).CloseTime.Before(got.Bar(1).CloseTime))

	_, err = market.NewDataset(id, market.Day, 7, append(bars, bars[0]))
	assert.ErrorIs(t, err, market.ErrDuplicateBar)

	bad := testBar(id, "2026-01-07", 100000, 99000, 98000, 100000)
	_, err = market.NewDataset(id, market.Day, 7, []market.Bar{bad})
	assert.ErrorIs(t, err, market.ErrInvalidOHLC)
}

func TestInstrumentIDRejectsAmbiguousCode(t *testing.T) {
	_, err := market.ParseInstrumentID("000001")
	assert.ErrorIs(t, err, market.ErrExchangeRequired)
}

func TestInstrumentIDParsesAndValidatesExchangeQualifiedCodes(t *testing.T) {
	id, err := market.ParseInstrumentID(" sse:600000 ")
	require.NoError(t, err)
	assert.Equal(t, market.InstrumentID{Exchange: market.SSE, Code: "600000"}, id)
	assert.Equal(t, "SSE:600000", id.String())

	_, err = market.ParseInstrumentID("SSE:600000:extra")
	assert.ErrorIs(t, err, market.ErrInvalidInstrument)

	_, err = market.ParseInstrumentID("OTHER:600000")
	assert.ErrorIs(t, err, market.ErrInvalidExchange)

	_, err = market.ParseInstrumentID("SSE:60000A")
	assert.ErrorIs(t, err, market.ErrInvalidInstrumentCode)

	err = (market.InstrumentID{Exchange: market.SSE, Code: "60000"}).Validate()
	assert.ErrorIs(t, err, market.ErrInvalidInstrumentCode)
}

func TestNewDatasetValidatesMetadataAndDefensivelyCopiesBars(t *testing.T) {
	id := market.InstrumentID{Exchange: market.SSE, Code: "600000"}
	bar := testBar(id, "2026-01-05", 100000, 103000, 99000, 102000)
	bar.Version = 1
	limitUp := market.Price(110000)
	bar.LimitUp = &limitUp

	_, err := market.NewDataset(market.InstrumentID{}, market.Day, 1, []market.Bar{bar})
	assert.ErrorIs(t, err, market.ErrInvalidInstrument)

	_, err = market.NewDataset(id, market.UnknownTimeframe, 1, []market.Bar{bar})
	assert.ErrorIs(t, err, market.ErrInvalidTimeframe)

	wrongInstrument := bar
	wrongInstrument.Instrument = market.InstrumentID{Exchange: market.SZSE, Code: "000001"}
	_, err = market.NewDataset(id, market.Day, 1, []market.Bar{wrongInstrument})
	assert.ErrorIs(t, err, market.ErrBarInstrumentMismatch)

	wrongTimeframe := bar
	wrongTimeframe.Timeframe = market.Week
	_, err = market.NewDataset(id, market.Day, 1, []market.Bar{wrongTimeframe})
	assert.ErrorIs(t, err, market.ErrBarTimeframeMismatch)

	wrongVersion := bar
	wrongVersion.Version = 2
	_, err = market.NewDataset(id, market.Day, 1, []market.Bar{wrongVersion})
	assert.ErrorIs(t, err, market.ErrBarVersionMismatch)

	negativeVolume := bar
	negativeVolume.Volume = -1
	_, err = market.NewDataset(id, market.Day, 1, []market.Bar{negativeVolume})
	assert.ErrorIs(t, err, market.ErrNegativeVolume)

	got, err := market.NewDataset(id, market.Day, 1, []market.Bar{bar})
	require.NoError(t, err)
	assert.Equal(t, id, got.Instrument())
	assert.Equal(t, market.Day, got.Timeframe())
	assert.Equal(t, market.DataVersion(1), got.Version())
	assert.Equal(t, 1, got.Len())
	copyOfBars := got.Bars()
	copyOfBars[0].Close = 1
	*copyOfBars[0].LimitUp = 1
	assert.Equal(t, market.Price(102000), got.Bar(0).Close)
	assert.Equal(t, market.Price(110000), *got.Bar(0).LimitUp)
}

func testBar(id market.InstrumentID, date string, open, high, low, close market.Price) market.Bar {
	closeTime := utc(date)
	return market.Bar{
		Instrument: id,
		Timeframe:  market.Day,
		OpenTime:   closeTime.Add(-24 * time.Hour),
		CloseTime:  closeTime,
		Open:       open,
		High:       high,
		Low:        low,
		Close:      close,
		Volume:     100,
		Amount:     10200000,
		Trading:    market.Tradable,
		Version:    7,
	}
}

func utc(date string) time.Time {
	value, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return value.UTC()
}
