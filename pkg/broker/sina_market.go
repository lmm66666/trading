package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
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

const (
	sinaDailyURL      = "https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"
	sinaFactorURL     = "https://finance.sina.com.cn/realstock/company"
	sinaResponseLimit = 4 << 20
)

type SinaMarketSource struct {
	client        *http.Client
	limiter       *rate.Limiter
	dailyBaseURL  string
	factorBaseURL string
}

var _ port.EquityDailySource = (*SinaMarketSource)(nil)

func NewSinaMarketSource(limiter *rate.Limiter) *SinaMarketSource {
	return NewSinaMarketSourceWithClient(nil, limiter, "", "")
}

func NewSinaMarketSourceWithClient(client *http.Client, limiter *rate.Limiter, dailyBaseURL, factorBaseURL string) *SinaMarketSource {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	} else {
		copied := *client
		client = &copied
		if client.Timeout <= 0 {
			client.Timeout = 15 * time.Second
		}
	}
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Every(5*time.Second), 1)
	}
	if dailyBaseURL == "" {
		dailyBaseURL = sinaDailyURL
	}
	if factorBaseURL == "" {
		factorBaseURL = sinaFactorURL
	}
	return &SinaMarketSource{client: client, limiter: limiter, dailyBaseURL: strings.TrimRight(dailyBaseURL, "/"), factorBaseURL: strings.TrimRight(factorBaseURL, "/")}
}

func (s *SinaMarketSource) FetchDailyBars(ctx context.Context, id market.InstrumentID, from, to time.Time) ([]market.Bar, error) {
	symbol, err := sinaSymbol(id)
	if err != nil || from.IsZero() || to.IsZero() || from.After(to) {
		return nil, fmt.Errorf("%w: invalid daily request", ErrInvalidRequest)
	}
	endpoint, err := url.Parse(s.dailyBaseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("%w: invalid daily endpoint", ErrInvalidRequest)
	}
	query := endpoint.Query()
	query.Set("symbol", symbol)
	query.Set("scale", "240")
	query.Set("ma", "no")
	query.Set("datalen", strconv.Itoa(sinaDataLength(from, to)))
	endpoint.RawQuery = query.Encode()
	body, err := s.get(ctx, endpoint.String())
	if err != nil {
		return nil, err
	}
	return ParseSinaDaily(body, id, from, to)
}

func (s *SinaMarketSource) FetchAdjustmentFactors(ctx context.Context, id market.InstrumentID) ([]market.AdjustmentFactor, error) {
	symbol, err := sinaSymbol(id)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(s.factorBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid factor endpoint", ErrInvalidRequest)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + symbol + "/qfq.js"
	body, err := s.get(ctx, base.String())
	if err != nil {
		return nil, err
	}
	return ParseSinaQFQ(body)
}

func (s *SinaMarketSource) get(ctx context.Context, endpoint string) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, classifyEastmoneyRequestError(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: create request", ErrInvalidRequest)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Referer", "https://finance.sina.com.cn/")
		req.Header.Set("User-Agent", "trading/1.0")
		resp, err := s.client.Do(req)
		if err != nil {
			last = classifyEastmoneyRequestError(err)
			if ctx.Err() != nil || !retryableSinaError(last) || attempt == 2 {
				return nil, last
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, sinaResponseLimit+1))
		_ = resp.Body.Close()
		if readErr != nil {
			last = classifyEastmoneyRequestError(readErr)
		} else if len(body) > sinaResponseLimit {
			return nil, fmt.Errorf("%w: body exceeds limit", ErrMalformedResponse)
		} else if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return body, nil
		} else {
			last = upstreamError(ErrUpstream, nil, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
		if attempt == 2 || !retryableSinaStatus(resp.StatusCode) {
			return nil, last
		}
	}
	return nil, last
}

func retryableSinaStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryableSinaError(err error) bool {
	return !errors.Is(err, ErrRequestCanceled) && (errors.Is(err, ErrUpstreamTimeout) || errors.Is(err, ErrUpstream))
}

func sinaSymbol(id market.InstrumentID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("%w: invalid instrument", ErrInvalidRequest)
	}
	switch id.Exchange {
	case market.SSE:
		return "sh" + id.Code, nil
	case market.SZSE:
		return "sz" + id.Code, nil
	case market.BSE:
		return "bj" + id.Code, nil
	default:
		return "", fmt.Errorf("%w: unsupported exchange", ErrInvalidRequest)
	}
}

func sinaDataLength(from, to time.Time) int {
	days := int(utcDate(to).Sub(utcDate(from)).Hours()/24) + 1
	length := days*2 + 20
	if length < 30 {
		return 30
	}
	if length > 10_000 {
		return 10_000
	}
	return length
}

type sinaDailyRow struct {
	Day    string `json:"day"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

func ParseSinaDaily(body []byte, id market.InstrumentID, from, to time.Time) ([]market.Bar, error) {
	if err := id.Validate(); err != nil || from.IsZero() || to.IsZero() || from.After(to) {
		return nil, fmt.Errorf("%w: invalid daily request", ErrMalformedResponse)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var rows []sinaDailyRow
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("%w: invalid daily response", ErrMalformedResponse)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("%w: multiple daily values", ErrMalformedResponse)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: empty daily response", ErrIncompleteData)
	}
	from = utcDate(from)
	to = utcDate(to)
	bars := make([]market.Bar, 0, len(rows))
	seen := make(map[time.Time]struct{}, len(rows))
	for _, row := range rows {
		at, err := time.Parse("2006-01-02", row.Day)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid trade date", ErrMalformedResponse)
		}
		at = at.UTC()
		if _, duplicate := seen[at]; duplicate {
			return nil, fmt.Errorf("%w: duplicate trade date", ErrMalformedResponse)
		}
		seen[at] = struct{}{}
		if at.Before(from) {
			continue
		}
		if at.After(to) {
			return nil, fmt.Errorf("%w: daily response after requested range", ErrIncompleteData)
		}
		open, err := parseEastmoneyScaled(strings.TrimSpace(row.Open), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid open", ErrMalformedResponse)
		}
		high, err := parseEastmoneyScaled(strings.TrimSpace(row.High), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid high", ErrMalformedResponse)
		}
		low, err := parseEastmoneyScaled(strings.TrimSpace(row.Low), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid low", ErrMalformedResponse)
		}
		closePrice, err := parseEastmoneyScaled(strings.TrimSpace(row.Close), market.ValueScale)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid close", ErrMalformedResponse)
		}
		volume, err := strconv.ParseInt(strings.TrimSpace(row.Volume), 10, 64)
		if err != nil || volume < 0 {
			return nil, fmt.Errorf("%w: invalid volume", ErrMalformedResponse)
		}
		bars = append(bars, market.Bar{
			Instrument: id,
			Timeframe:  market.Day,
			OpenTime:   at,
			CloseTime:  at,
			Open:       market.Price(open),
			High:       market.Price(high),
			Low:        market.Price(low),
			Close:      market.Price(closePrice),
			Volume:     volume,
			Trading:    market.Tradable,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("%w: no daily rows in requested range", ErrIncompleteData)
	}
	dataset, err := market.NewDataset(id, market.Day, 0, bars)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid daily bars: %v", ErrMalformedResponse, err)
	}
	return dataset.Bars(), nil
}

type sinaQFQEnvelope struct {
	Total int          `json:"total"`
	Data  []sinaQFQRow `json:"data"`
}

type sinaQFQRow struct {
	Date   string `json:"d"`
	Factor string `json:"f"`
}

func ParseSinaQFQ(body []byte) ([]market.AdjustmentFactor, error) {
	text := strings.TrimSpace(string(body))
	if !strings.HasPrefix(text, "var ") {
		return nil, fmt.Errorf("%w: invalid qfq prefix", ErrMalformedResponse)
	}
	equals := strings.IndexByte(text, '=')
	if equals < 5 || !strings.HasSuffix(strings.TrimSpace(text[4:equals]), "qfq") {
		return nil, fmt.Errorf("%w: invalid qfq variable", ErrMalformedResponse)
	}
	payload := strings.TrimSpace(text[equals+1:])
	payload = strings.TrimSuffix(payload, ";")
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope sinaQFQEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: invalid qfq response", ErrMalformedResponse)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("%w: multiple qfq values", ErrMalformedResponse)
	}
	if envelope.Total != len(envelope.Data) {
		return nil, fmt.Errorf("%w: qfq total mismatch", ErrIncompleteData)
	}
	if len(envelope.Data) == 0 {
		return nil, fmt.Errorf("%w: empty qfq response", ErrIncompleteData)
	}
	factors := make([]market.AdjustmentFactor, 0, len(envelope.Data))
	seen := make(map[time.Time]struct{}, len(envelope.Data))
	for _, row := range envelope.Data {
		at, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid qfq date", ErrMalformedResponse)
		}
		at = at.UTC()
		if _, duplicate := seen[at]; duplicate {
			return nil, fmt.Errorf("%w: duplicate qfq date", ErrMalformedResponse)
		}
		seen[at] = struct{}{}
		factor, ok := new(big.Rat).SetString(strings.TrimSpace(row.Factor))
		if !ok || factor.Sign() <= 0 {
			return nil, fmt.Errorf("%w: invalid qfq factor", ErrMalformedResponse)
		}
		factor.Inv(factor)
		if !factor.Num().IsInt64() || !factor.Denom().IsInt64() {
			return nil, fmt.Errorf("%w: qfq factor overflow", ErrMalformedResponse)
		}
		factors = append(factors, market.AdjustmentFactor{
			EffectiveTime: at,
			Numerator:     factor.Num().Int64(),
			Denominator:   factor.Denom().Int64(),
		})
	}
	sort.Slice(factors, func(i, j int) bool { return factors[i].EffectiveTime.Before(factors[j].EffectiveTime) })
	return factors, nil
}

func utcDate(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
