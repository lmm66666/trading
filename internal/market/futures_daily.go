package market

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidFuturesDaily = errors.New("market: invalid futures daily observation")

const ExchangeDailyStatistics = "EXCHANGE_DAILY"

type FuturesDaily struct {
	Bar              Bar
	Instrument       InstrumentID
	TradeDate        time.Time
	PreSettlement    Price
	Settlement       Price
	VolumeLots       int64
	OpenInterestLots int64
	Turnover         Money
	RawSymbol        string
	Product          string
	StatisticsBasis  string
	ProviderVersion  string
}

func (daily FuturesDaily) Validate() error {
	if err := daily.Instrument.Validate(); err != nil || daily.Instrument.Kind() != FuturesContract {
		return fmt.Errorf("%w: contract instrument is required", ErrInvalidFuturesDaily)
	}
	if !isUTCDate(daily.TradeDate) {
		return fmt.Errorf("%w: trade date must be UTC midnight", ErrInvalidFuturesDaily)
	}
	if daily.Bar.Instrument != daily.Instrument || daily.Bar.Timeframe != Day || daily.Bar.Version != 0 {
		return fmt.Errorf("%w: bar identity is inconsistent", ErrInvalidFuturesDaily)
	}
	if daily.Bar.OpenTime.Location() != time.UTC || daily.Bar.CloseTime.Location() != time.UTC || daily.Bar.OpenTime.IsZero() || daily.Bar.CloseTime.Before(daily.Bar.OpenTime) {
		return fmt.Errorf("%w: invalid bar time", ErrInvalidFuturesDaily)
	}
	closeDate := time.Date(daily.Bar.CloseTime.Year(), daily.Bar.CloseTime.Month(), daily.Bar.CloseTime.Day(), 0, 0, 0, 0, time.UTC)
	if closeDate != daily.TradeDate {
		return fmt.Errorf("%w: bar does not belong to trade date", ErrInvalidFuturesDaily)
	}
	if _, err := NewDataset(daily.Instrument, Day, 0, []Bar{daily.Bar}); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFuturesDaily, err)
	}
	if daily.PreSettlement <= 0 || daily.Settlement <= 0 || daily.VolumeLots < 0 || daily.OpenInterestLots < 0 || daily.Turnover < 0 {
		return fmt.Errorf("%w: invalid settlement, volume, open interest, or turnover", ErrInvalidFuturesDaily)
	}
	if daily.Bar.Volume != daily.VolumeLots || daily.Bar.Amount != daily.Turnover {
		return fmt.Errorf("%w: common and futures fields disagree", ErrInvalidFuturesDaily)
	}
	if !validRawFuturesSymbol(daily.RawSymbol) || daily.Product != daily.Instrument.Product() {
		return fmt.Errorf("%w: invalid symbol or product", ErrInvalidFuturesDaily)
	}
	if daily.StatisticsBasis != ExchangeDailyStatistics || !validMetadata(daily.ProviderVersion, 32) {
		return fmt.Errorf("%w: invalid provider metadata", ErrInvalidFuturesDaily)
	}
	return nil
}

func isUTCDate(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func validRawFuturesSymbol(value string) bool {
	if !validMetadata(value, 32) {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
			return false
		}
	}
	switch strings.ToUpper(value) {
	case "0", "88", "888", "99":
		return false
	default:
		return true
	}
}

func validMetadata(value string, maxBytes int) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && len(value) <= maxBytes
}

type MainChangeReason string

const (
	MainInitial          MainChangeReason = "INITIAL"
	MainOpenInterest     MainChangeReason = "OPEN_INTEREST"
	MainFastOpenInterest MainChangeReason = "FAST_OPEN_INTEREST"
	MainForcedExpiry     MainChangeReason = "FORCED_EXPIRY"
	MainNoTrade          MainChangeReason = "NO_TRADE"
)

type MainMapping struct {
	Continuous      InstrumentID
	TradeDate       time.Time
	Contract        InstrumentID
	DecisionDate    time.Time
	Reason          MainChangeReason
	OldOpenInterest int64
	NewOpenInterest int64
	OldVolume       int64
	NewVolume       int64
}
