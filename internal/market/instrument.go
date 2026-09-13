package market

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrExchangeRequired      = errors.New("market: exchange required")
	ErrInvalidInstrument     = errors.New("market: invalid instrument")
	ErrInvalidExchange       = errors.New("market: invalid exchange")
	ErrInvalidInstrumentCode = errors.New("market: invalid instrument code")
)

type Exchange string

const (
	SSE  Exchange = "SSE"
	SZSE Exchange = "SZSE"
	BSE  Exchange = "BSE"
	SHFE Exchange = "SHFE"
	INE  Exchange = "INE"
	DCE  Exchange = "DCE"
	CZCE Exchange = "CZCE"
)

type AssetClass string

const (
	Equity  AssetClass = "EQUITY"
	Futures AssetClass = "FUTURES"
)

type InstrumentKind string

const (
	SpotEquity        InstrumentKind = "SPOT_EQUITY"
	FuturesContract   InstrumentKind = "FUTURES_CONTRACT"
	FuturesContinuous InstrumentKind = "FUTURES_CONTINUOUS"
)

var (
	futuresContractCode   = regexp.MustCompile(`^([A-Z]{1,2})(20[0-9]{2})(0[1-9]|1[0-2])$`)
	futuresContinuousCode = regexp.MustCompile(`^([A-Z]{1,2})\.MAIN$`)
	futuresProducts       = map[Exchange]map[string]struct{}{
		SHFE: {"AU": {}, "AG": {}, "FU": {}},
		INE:  {"SC": {}, "LU": {}},
		DCE:  {"J": {}, "JM": {}},
		CZCE: {"ZC": {}},
	}
)

type InstrumentID struct {
	Exchange Exchange
	Code     string
}

func ParseInstrumentID(value string) (InstrumentID, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 1 {
		return InstrumentID{}, ErrExchangeRequired
	}
	if len(parts) != 2 {
		return InstrumentID{}, ErrInvalidInstrument
	}

	id := InstrumentID{
		Exchange: Exchange(strings.ToUpper(strings.TrimSpace(parts[0]))),
		Code:     strings.TrimSpace(parts[1]),
	}
	if err := id.Validate(); err != nil {
		return InstrumentID{}, err
	}
	return id, nil
}

func (id InstrumentID) Validate() error {
	switch id.Exchange {
	case SSE, SZSE, BSE:
		if len(id.Code) != 6 {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
		}
		for _, char := range id.Code {
			if char < '0' || char > '9' {
				return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
			}
		}
		return nil
	case SHFE, INE, DCE, CZCE:
		product := id.Product()
		if product == "" {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
		}
		if _, allowed := futuresProducts[id.Exchange][product]; !allowed {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
		}
		return nil
	default:
		if id.Exchange == "" {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrExchangeRequired)
		}
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidExchange)
	}

}

func (id InstrumentID) String() string {
	return string(id.Exchange) + ":" + id.Code
}

func (id InstrumentID) AssetClass() AssetClass {
	switch id.Exchange {
	case SSE, SZSE, BSE:
		return Equity
	case SHFE, INE, DCE, CZCE:
		return Futures
	default:
		return ""
	}
}

func (id InstrumentID) Kind() InstrumentKind {
	if id.AssetClass() == Equity {
		return SpotEquity
	}
	if futuresContractCode.MatchString(id.Code) {
		return FuturesContract
	}
	if futuresContinuousCode.MatchString(id.Code) {
		return FuturesContinuous
	}
	return ""
}

func (id InstrumentID) Product() string {
	if matches := futuresContractCode.FindStringSubmatch(id.Code); len(matches) == 4 {
		return matches[1]
	}
	if matches := futuresContinuousCode.FindStringSubmatch(id.Code); len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func (id InstrumentID) DeliveryMonth() (time.Time, bool) {
	matches := futuresContractCode.FindStringSubmatch(id.Code)
	if len(matches) != 4 {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(matches[2])
	month, _ := strconv.Atoi(matches[3])
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), true
}
