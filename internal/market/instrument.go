package market

import (
	"errors"
	"fmt"
	"strings"
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
	default:
		if id.Exchange == "" {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrExchangeRequired)
		}
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidExchange)
	}

	if len(id.Code) != 6 {
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
	}
	for _, char := range id.Code {
		if char < '0' || char > '9' {
			return fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidInstrumentCode)
		}
	}
	return nil
}

func (id InstrumentID) String() string {
	return string(id.Exchange) + ":" + id.Code
}
