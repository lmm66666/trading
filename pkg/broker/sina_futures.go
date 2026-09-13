package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"trading/internal/market"
	"trading/internal/port"
)

const sinaFuturesDailyURL = "https://stock2.finance.sina.com.cn/futures/api/jsonp.php"

type SinaFuturesSource struct {
	transport *SinaMarketSource
	baseURL   string
}

var _ port.DailyMarketSource = (*SinaFuturesSource)(nil)

func NewSinaFuturesSource(limiter *rate.Limiter) *SinaFuturesSource {
	return NewSinaFuturesSourceWithClient(nil, limiter, "")
}

func NewSinaFuturesSourceWithClient(client *http.Client, limiter *rate.Limiter, baseURL string) *SinaFuturesSource {
	if baseURL == "" {
		baseURL = sinaFuturesDailyURL
	}
	return &SinaFuturesSource{
		transport: NewSinaMarketSourceWithClient(client, limiter, "", ""),
		baseURL:   strings.TrimRight(baseURL, "/"),
	}
}

func (source *SinaFuturesSource) FetchDailyBars(ctx context.Context, id market.InstrumentID, from, to time.Time) ([]market.Bar, error) {
	symbol, err := sinaFuturesSymbol(id)
	if err != nil || from.IsZero() || to.IsZero() || from.After(to) {
		return nil, fmt.Errorf("%w: invalid futures daily request", ErrInvalidRequest)
	}
	base, err := url.Parse(source.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid futures endpoint", ErrInvalidRequest)
	}
	dateToken := fmt.Sprintf("%d_%d_%d", to.UTC().Year(), int(to.UTC().Month()), to.UTC().Day())
	variable := symbol + dateToken
	endpoint, err := url.Parse(source.baseURL + "/var%20_" + variable + "=/InnerFuturesNewService.getDailyKLine")
	if err != nil {
		return nil, fmt.Errorf("%w: invalid futures endpoint", ErrInvalidRequest)
	}
	query := endpoint.Query()
	query.Set("symbol", symbol)
	query.Set("type", dateToken)
	endpoint.RawQuery = query.Encode()
	body, err := source.transport.get(ctx, endpoint.String())
	if err != nil {
		return nil, err
	}
	return ParseSinaFuturesDaily(body, id, from, to)
}

func (source *SinaFuturesSource) FetchAdjustmentFactors(_ context.Context, id market.InstrumentID) ([]market.AdjustmentFactor, error) {
	if _, err := sinaFuturesSymbol(id); err != nil {
		return nil, err
	}
	return []market.AdjustmentFactor{{
		EffectiveTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		Numerator:     1,
		Denominator:   1,
	}}, nil
}

func sinaFuturesSymbol(id market.InstrumentID) (string, error) {
	if err := id.Validate(); err != nil || id.Kind() != market.FuturesContinuous {
		return "", fmt.Errorf("%w: unsupported futures instrument", ErrInvalidRequest)
	}
	allowed := map[market.Exchange]map[string]bool{
		market.SHFE: {"AU": true, "AG": true, "FU": true},
		market.INE:  {"SC": true, "LU": true},
		market.DCE:  {"J": true, "JM": true},
		market.CZCE: {"ZC": true},
	}
	product := id.Product()
	if !allowed[id.Exchange][product] {
		return "", fmt.Errorf("%w: unsupported futures product", ErrInvalidRequest)
	}
	return product + "0", nil
}

type sinaFuturesDailyRow struct {
	Date         string `json:"d"`
	Open         string `json:"o"`
	High         string `json:"h"`
	Low          string `json:"l"`
	Close        string `json:"c"`
	Volume       string `json:"v"`
	OpenInterest string `json:"p"`
	Settlement   string `json:"s"`
}

func ParseSinaFuturesDaily(body []byte, id market.InstrumentID, from, to time.Time) ([]market.Bar, error) {
	symbol, err := sinaFuturesSymbol(id)
	if err != nil || from.IsZero() || to.IsZero() || from.After(to) {
		return nil, fmt.Errorf("%w: invalid futures daily request", ErrMalformedResponse)
	}
	text := bytes.TrimSpace(body)
	if bytes.HasPrefix(text, []byte("/*")) {
		commentEnd := bytes.Index(text, []byte("*/"))
		if commentEnd < 0 {
			return nil, fmt.Errorf("%w: invalid futures JSONP comment", ErrMalformedResponse)
		}
		text = bytes.TrimSpace(text[commentEnd+2:])
	}
	dateToken := fmt.Sprintf("%d_%d_%d", to.UTC().Year(), int(to.UTC().Month()), to.UTC().Day())
	prefix := []byte("var _" + symbol + dateToken + "=(")
	if !bytes.HasPrefix(text, prefix) || !bytes.HasSuffix(text, []byte(");")) {
		return nil, fmt.Errorf("%w: invalid futures JSONP", ErrMalformedResponse)
	}
	payload := bytes.TrimSpace(text[len(prefix) : len(text)-2])
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var rows []sinaFuturesDailyRow
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("%w: invalid futures rows", ErrMalformedResponse)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%w: multiple futures values", ErrMalformedResponse)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty futures response", ErrIncompleteData)
	}
	fromDate, toDate := utcDate(from), utcDate(to)
	bars := make([]market.Bar, 0, len(rows))
	seen := make(map[time.Time]struct{}, len(rows))
	for _, row := range rows {
		tradeDate, err := time.Parse(time.DateOnly, row.Date)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid futures date", ErrMalformedResponse)
		}
		tradeDate = tradeDate.UTC()
		if _, duplicate := seen[tradeDate]; duplicate {
			return nil, fmt.Errorf("%w: duplicate futures date", ErrMalformedResponse)
		}
		seen[tradeDate] = struct{}{}
		if tradeDate.After(toDate) {
			return nil, fmt.Errorf("%w: futures response after requested range", ErrIncompleteData)
		}
		if tradeDate.Before(fromDate) {
			continue
		}
		open, err := parseEastmoneyScaled(strings.TrimSpace(row.Open), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid futures open", ErrMalformedResponse)
		}
		high, err := parseEastmoneyScaled(strings.TrimSpace(row.High), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid futures high", ErrMalformedResponse)
		}
		low, err := parseEastmoneyScaled(strings.TrimSpace(row.Low), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid futures low", ErrMalformedResponse)
		}
		closePrice, err := parseEastmoneyScaled(strings.TrimSpace(row.Close), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid futures close", ErrMalformedResponse)
		}
		volume, err := strconv.ParseInt(strings.TrimSpace(row.Volume), 10, 64)
		if err != nil || volume < 0 {
			return nil, fmt.Errorf("%w: invalid futures volume", ErrMalformedResponse)
		}
		openInterest, oiErr := strconv.ParseInt(strings.TrimSpace(row.OpenInterest), 10, 64)
		settlement, settlementErr := parseEastmoneyScaled(strings.TrimSpace(row.Settlement), market.ValueScale)
		if oiErr != nil || openInterest < 0 || settlementErr != nil || settlement < 0 {
			return nil, fmt.Errorf("%w: invalid futures metadata", ErrMalformedResponse)
		}
		bar := market.Bar{
			Instrument: id,
			Timeframe:  market.Day,
			OpenTime:   tradeDate.Add(-11 * time.Hour),
			CloseTime:  tradeDate.Add(7 * time.Hour),
			Open:       market.Price(open),
			High:       market.Price(high),
			Low:        market.Price(low),
			Close:      market.Price(closePrice),
			Volume:     volume,
			Trading:    market.Tradable,
		}
		if _, err := market.NewDataset(id, market.Day, 0, []market.Bar{bar}); err != nil {
			return nil, fmt.Errorf("%w: invalid futures OHLC", ErrMalformedResponse)
		}
		bars = append(bars, bar)
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].CloseTime.Before(bars[j].CloseTime) })
	return bars, nil
}
