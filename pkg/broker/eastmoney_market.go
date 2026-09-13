package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"trading/internal/market"
	"trading/internal/port"
)

const (
	eastmoneyKlineURL      = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	eastmoneyDataCenterURL = "https://datacenter-web.eastmoney.com/api/data/v1/get"
	eastmoneyResponseLimit = 4 << 20
	eastmoneyMaxPages      = 10_000
)

var (
	eastmoneyHTTPTimeout = 15 * time.Second
	decimalPattern       = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
)

// EastmoneyMarketSource provides raw bars, QFQ factors, and corporate actions
// for the versioned market-data port. It never retries requests: retries are an
// application-service policy so a failed source call remains observable.
type EastmoneyMarketSource struct {
	client            *http.Client
	klineBaseURL      string
	dataCenterBaseURL string
}

var _ port.MarketSource = (*EastmoneyMarketSource)(nil)

func NewEastmoneyMarketSource() *EastmoneyMarketSource {
	return NewEastmoneyMarketSourceWithClient(nil, "", "")
}

// NewEastmoneyMarketSourceWithClient permits deterministic fixture servers.
// Empty URLs select Eastmoney's production endpoints; a nil client has an
// explicit bounded timeout.
func NewEastmoneyMarketSourceWithClient(client *http.Client, klineBaseURL, dataCenterBaseURL string) *EastmoneyMarketSource {
	if client == nil {
		client = &http.Client{Timeout: eastmoneyHTTPTimeout}
	}
	if klineBaseURL == "" {
		klineBaseURL = eastmoneyKlineURL
	}
	if dataCenterBaseURL == "" {
		dataCenterBaseURL = eastmoneyDataCenterURL
	}
	return &EastmoneyMarketSource{
		client:            client,
		klineBaseURL:      strings.TrimRight(klineBaseURL, "/"),
		dataCenterBaseURL: strings.TrimRight(dataCenterBaseURL, "/"),
	}
}

func (s *EastmoneyMarketSource) FetchBars(ctx context.Context, id market.InstrumentID, tf market.Timeframe, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	if err := validateEastmoneyBarRequest(id, tf, from, to); err != nil {
		return nil, nil, err
	}
	secid, err := eastmoneySecurityID(id)
	if err != nil {
		return nil, nil, err
	}

	query := url.Values{
		"secid":   {secid},
		"klt":     {eastmoneyKlinePeriod(tf)},
		"beg":     {from.In(time.UTC).Format("20060102")},
		"end":     {to.In(time.UTC).Format("20060102")},
		"fields1": {"f1,f2,f3,f4,f5,f6"},
		"fields2": {"f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61"},
	}
	rawQuery := cloneValues(query)
	rawQuery.Set("fqt", "0")
	raw, err := s.get(ctx, s.klineBaseURL, rawQuery)
	if err != nil {
		return nil, nil, err
	}
	qfqQuery := cloneValues(query)
	qfqQuery.Set("fqt", "1")
	qfq, err := s.get(ctx, s.klineBaseURL, qfqQuery)
	if err != nil {
		return nil, nil, err
	}

	bars, factors, err := ParseEastmoneyKlines(raw, qfq, id, tf)
	if err != nil {
		return nil, nil, err
	}
	return filterEastmoneyRange(bars, factors, from, to)
}

func (s *EastmoneyMarketSource) FetchCorporateActions(ctx context.Context, id market.InstrumentID) ([]market.CorporateAction, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid instrument: %v", ErrMalformedResponse, err)
	}

	var actions []market.CorporateAction
	pages := 1
	for page := 1; page <= pages; page++ {
		query := url.Values{
			"reportName":  {"RPT_SHAREBONUS_DET"},
			"columns":     {"SECURITY_CODE,EX_DIVIDEND_DATE,PRETAX_BONUS_RMB,BONUS_RATIO,IT_RATIO"},
			"filter":      {fmt.Sprintf(`(SECURITY_CODE="%s")`, id.Code)},
			"pageNumber":  {strconv.Itoa(page)},
			"pageSize":    {"500"},
			"sortColumns": {"EX_DIVIDEND_DATE"},
			"sortTypes":   {"1"},
			"source":      {"WEB"},
			"client":      {"WEB"},
		}
		body, err := s.get(ctx, s.dataCenterBaseURL, query)
		if err != nil {
			return nil, err
		}
		pageActions, responsePages, err := parseEastmoneyCorporateActionPage(body, id)
		if err != nil {
			return nil, err
		}
		if page == 1 {
			pages = responsePages
			if pages > eastmoneyMaxPages {
				return nil, fmt.Errorf("%w: pages exceeds limit", ErrMalformedResponse)
			}
		} else if responsePages != pages {
			return nil, fmt.Errorf("%w: pagination changed from %d to %d", ErrMalformedResponse, pages, responsePages)
		}
		actions = append(actions, pageActions...)
	}

	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].ExDate.Equal(actions[j].ExDate) {
			if actions[i].Kind == actions[j].Kind {
				return actions[i].ID < actions[j].ID
			}
			return actions[i].Kind < actions[j].Kind
		}
		return actions[i].ExDate.Before(actions[j].ExDate)
	})
	for i := 1; i < len(actions); i++ {
		if actions[i].ID == actions[i-1].ID {
			return nil, fmt.Errorf("%w: duplicate action %q", ErrMalformedResponse, actions[i].ID)
		}
	}
	return actions, nil
}

func (s *EastmoneyMarketSource) get(ctx context.Context, baseURL string, query url.Values) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, upstreamError(ErrRequestCanceled, err, 0, "")
	}
	endpoint := baseURL + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request", ErrMalformedResponse)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, classifyEastmoneyRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, upstreamError(ErrUpstream, nil, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, eastmoneyResponseLimit+1))
	if err != nil {
		return nil, classifyEastmoneyRequestError(err)
	}
	if len(body) > eastmoneyResponseLimit {
		return nil, fmt.Errorf("%w: body exceeds %d bytes", ErrMalformedResponse, eastmoneyResponseLimit)
	}
	return body, nil
}

func ParseEastmoneyKlines(rawBody, qfqBody []byte, id market.InstrumentID, tf market.Timeframe) ([]market.Bar, []market.AdjustmentFactor, error) {
	if err := id.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid instrument: %v", ErrMalformedResponse, err)
	}
	if !tf.Valid() {
		return nil, nil, fmt.Errorf("%w: invalid timeframe", ErrMalformedResponse)
	}
	raw, err := parseEastmoneyKlineResponse(rawBody)
	if err != nil {
		return nil, nil, err
	}
	qfq, err := parseEastmoneyKlineResponse(qfqBody)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) == 0 || len(qfq) == 0 || len(raw) != len(qfq) {
		return nil, nil, fmt.Errorf("%w: raw and QFQ series differ in length", ErrIncompleteData)
	}
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].time.Before(raw[j].time) })
	sort.SliceStable(qfq, func(i, j int) bool { return qfq[i].time.Before(qfq[j].time) })

	qfqByDate := make(map[string]eastmoneyKline, len(qfq))
	for _, line := range qfq {
		if _, exists := qfqByDate[line.date]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate QFQ date %s", ErrIncompleteData, line.date)
		}
		qfqByDate[line.date] = line
	}
	bars := make([]market.Bar, 0, len(raw))
	factors := make([]market.AdjustmentFactor, 0, len(raw))
	seenRaw := make(map[string]struct{}, len(raw))
	for _, line := range raw {
		if _, exists := seenRaw[line.date]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate raw date %s", ErrIncompleteData, line.date)
		}
		seenRaw[line.date] = struct{}{}
		adjusted, exists := qfqByDate[line.date]
		if !exists {
			return nil, nil, fmt.Errorf("%w: QFQ date missing for %s", ErrIncompleteData, line.date)
		}
		factor, err := eastmoneyFactor(line.close, adjusted.close)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrIncompleteData, err)
		}
		if !eastmoneyOHLCMatches(line, adjusted, factor) {
			return nil, nil, fmt.Errorf("%w: QFQ OHLC mismatch for %s", ErrIncompleteData, line.date)
		}
		closeTime := line.time
		bar := market.Bar{
			Instrument: id,
			Timeframe:  tf,
			OpenTime:   closeTime,
			CloseTime:  closeTime,
			Open:       line.open,
			High:       line.high,
			Low:        line.low,
			Close:      line.close,
			Volume:     line.volume,
			Amount:     line.amount,
			Trading:    line.trading,
			Version:    0,
		}
		bars = append(bars, bar)
		if len(factors) == 0 || factors[len(factors)-1].Numerator != factor.Numerator || factors[len(factors)-1].Denominator != factor.Denominator {
			factor.EffectiveTime = closeTime
			factors = append(factors, factor)
		}
	}
	if len(qfqByDate) != len(seenRaw) {
		return nil, nil, fmt.Errorf("%w: raw and QFQ dates differ", ErrIncompleteData)
	}
	sort.SliceStable(bars, func(i, j int) bool { return bars[i].CloseTime.Before(bars[j].CloseTime) })
	sort.SliceStable(factors, func(i, j int) bool { return factors[i].EffectiveTime.Before(factors[j].EffectiveTime) })
	return bars, factors, nil
}

func ParseEastmoneyCorporateActions(body []byte, id market.InstrumentID) ([]market.CorporateAction, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid instrument: %v", ErrMalformedResponse, err)
	}
	actions, _, err := parseEastmoneyCorporateActionPage(body, id)
	return actions, err
}

type eastmoneyKline struct {
	date                   string
	time                   time.Time
	open, high, low, close market.Price
	volume                 int64
	amount                 market.Money
	trading                market.TradingStatus
}

func parseEastmoneyKlineResponse(body []byte) ([]eastmoneyKline, error) {
	var response struct {
		Success *bool           `json:"success"`
		RC      *int            `json:"rc"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrMalformedResponse)
	}
	if response.Success != nil && !*response.Success {
		return nil, upstreamError(ErrUpstream, nil, 0, "")
	}
	if response.RC != nil && *response.RC != 0 {
		return nil, upstreamError(ErrUpstream, nil, 0, "")
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return nil, fmt.Errorf("%w: data is required", ErrMalformedResponse)
	}
	var data struct {
		Klines []string `json:"klines"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, fmt.Errorf("%w: invalid data", ErrMalformedResponse)
	}
	if data.Klines == nil {
		return nil, fmt.Errorf("%w: klines is required", ErrMalformedResponse)
	}
	lines := make([]eastmoneyKline, 0, len(data.Klines))
	for _, record := range data.Klines {
		line, err := parseEastmoneyKline(record)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func parseEastmoneyKline(record string) (eastmoneyKline, error) {
	fields := strings.Split(record, ",")
	if len(fields) < 7 {
		return eastmoneyKline{}, fmt.Errorf("%w: kline has %d fields", ErrMalformedResponse, len(fields))
	}
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(fields[0]), time.UTC)
	if err != nil {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid kline date", ErrMalformedResponse)
	}
	open, err := parseEastmoneyScaled(strings.TrimSpace(fields[1]), market.ValueScale)
	if err != nil {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid open", ErrMalformedResponse)
	}
	closePrice, err := parseEastmoneyScaled(strings.TrimSpace(fields[2]), market.ValueScale)
	if err != nil {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid close", ErrMalformedResponse)
	}
	high, err := parseEastmoneyScaled(strings.TrimSpace(fields[3]), market.ValueScale)
	if err != nil {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid high", ErrMalformedResponse)
	}
	low, err := parseEastmoneyScaled(strings.TrimSpace(fields[4]), market.ValueScale)
	if err != nil {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid low", ErrMalformedResponse)
	}
	volume, err := strconv.ParseInt(strings.TrimSpace(fields[5]), 10, 64)
	if err != nil || volume < 0 {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid volume", ErrMalformedResponse)
	}
	amount, err := parseEastmoneyScaled(strings.TrimSpace(fields[6]), market.ValueScale)
	if err != nil || amount < 0 {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid amount", ErrMalformedResponse)
	}
	if open <= 0 || closePrice <= 0 || high <= 0 || low <= 0 || low > open || low > closePrice || open > high || closePrice > high {
		return eastmoneyKline{}, fmt.Errorf("%w: invalid OHLC", ErrMalformedResponse)
	}
	if (volume == 0) != (amount == 0) {
		return eastmoneyKline{}, fmt.Errorf("%w: inconsistent suspended bar", ErrMalformedResponse)
	}
	trading := market.Tradable
	if volume == 0 {
		trading = market.Suspended
	}
	return eastmoneyKline{
		date:    date.Format("2006-01-02"),
		time:    date,
		open:    market.Price(open),
		close:   market.Price(closePrice),
		high:    market.Price(high),
		low:     market.Price(low),
		volume:  volume,
		amount:  market.Money(amount),
		trading: trading,
	}, nil
}

func eastmoneyFactor(raw, qfq market.Price) (market.AdjustmentFactor, error) {
	if raw <= 0 || qfq <= 0 {
		return market.AdjustmentFactor{}, errors.New("non-positive close")
	}
	ratio := new(big.Rat).SetFrac(big.NewInt(int64(qfq)), big.NewInt(int64(raw)))
	scaled := new(big.Rat).Mul(ratio, big.NewRat(100_000_000, 1))
	numerator := roundPositiveRat(scaled)
	if numerator.Sign() <= 0 || !numerator.IsInt64() {
		return market.AdjustmentFactor{}, errors.New("factor out of range")
	}
	denominator := int64(100_000_000)
	n := numerator.Int64()
	gcd := gcd64(n, denominator)
	return market.AdjustmentFactor{Numerator: n / gcd, Denominator: denominator / gcd, Version: 0}, nil
}

func eastmoneyOHLCMatches(raw, qfq eastmoneyKline, factor market.AdjustmentFactor) bool {
	for _, pair := range [][2]market.Price{{raw.open, qfq.open}, {raw.high, qfq.high}, {raw.low, qfq.low}, {raw.close, qfq.close}} {
		left := new(big.Int).Mul(big.NewInt(int64(pair[1])), big.NewInt(factor.Denominator))
		right := new(big.Int).Mul(big.NewInt(int64(pair[0])), big.NewInt(factor.Numerator))
		difference := new(big.Int).Sub(left, right)
		difference.Abs(difference)
		tolerance := new(big.Int).Mul(big.NewInt(market.ValueScale/100), big.NewInt(factor.Denominator))
		if difference.Cmp(tolerance) > 0 {
			return false
		}
	}
	return true
}

type eastmoneyCorporateActionRow struct {
	SecurityCode   json.RawMessage `json:"SECURITY_CODE"`
	ExDividendDate json.RawMessage `json:"EX_DIVIDEND_DATE"`
	PretaxBonus    json.RawMessage `json:"PRETAX_BONUS_RMB"`
	BonusRatio     json.RawMessage `json:"BONUS_RATIO"`
	ITRatio        json.RawMessage `json:"IT_RATIO"`
}

func parseEastmoneyCorporateActionPage(body []byte, id market.InstrumentID) ([]market.CorporateAction, int, error) {
	var envelope struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, fmt.Errorf("%w: invalid JSON", ErrMalformedResponse)
	}
	if envelope.Success == nil || !*envelope.Success {
		return nil, 0, upstreamError(ErrUpstream, nil, 0, "")
	}
	var code int
	if len(envelope.Code) == 0 || json.Unmarshal(envelope.Code, &code) != nil || code != 0 {
		if len(envelope.Code) > 0 && string(envelope.Code) != "0" {
			return nil, 0, upstreamError(ErrUpstream, nil, 0, "")
		}
		return nil, 0, fmt.Errorf("%w: invalid code", ErrMalformedResponse)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil, 0, fmt.Errorf("%w: result is required", ErrMalformedResponse)
	}
	var result struct {
		Pages *int            `json:"pages"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, 0, fmt.Errorf("%w: invalid result", ErrMalformedResponse)
	}
	if result.Pages == nil || *result.Pages < 0 || len(result.Data) == 0 {
		return nil, 0, fmt.Errorf("%w: pages and data are required", ErrMalformedResponse)
	}
	if string(result.Data) == "null" {
		return nil, *result.Pages, nil
	}
	var rows []eastmoneyCorporateActionRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		return nil, 0, fmt.Errorf("%w: invalid corporate actions", ErrMalformedResponse)
	}
	actions := make([]market.CorporateAction, 0, len(rows)*2)
	for _, row := range rows {
		rowActions, err := parseEastmoneyCorporateAction(row, id)
		if err != nil {
			return nil, 0, err
		}
		actions = append(actions, rowActions...)
	}
	return actions, *result.Pages, nil
}

func parseEastmoneyCorporateAction(row eastmoneyCorporateActionRow, id market.InstrumentID) ([]market.CorporateAction, error) {
	code, err := rawJSONString(row.SecurityCode)
	if err != nil || code != id.Code {
		return nil, fmt.Errorf("%w: invalid security code", ErrMalformedResponse)
	}
	exDateValue, err := rawJSONString(row.ExDividendDate)
	if err != nil || exDateValue == "" {
		return nil, fmt.Errorf("%w: ex-dividend date is required", ErrMalformedResponse)
	}
	exDate, err := parseEastmoneyActionDate(exDateValue)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid ex-dividend date", ErrMalformedResponse)
	}
	cash, err := rawJSONDecimal(row.PretaxBonus)
	if err != nil || cash.Sign() < 0 {
		return nil, fmt.Errorf("%w: invalid cash ratio", ErrMalformedResponse)
	}
	bonus, err := rawJSONDecimal(row.BonusRatio)
	if err != nil || bonus.Sign() < 0 {
		return nil, fmt.Errorf("%w: invalid bonus ratio", ErrMalformedResponse)
	}
	transfer, err := rawJSONDecimal(row.ITRatio)
	if err != nil || transfer.Sign() < 0 {
		return nil, fmt.Errorf("%w: invalid transfer ratio", ErrMalformedResponse)
	}

	actions := make([]market.CorporateAction, 0, 2)
	if cash.Sign() > 0 {
		cashPerShare := new(big.Rat).Quo(new(big.Rat).Mul(cash, big.NewRat(market.ValueScale, 1)), big.NewRat(10, 1))
		value, ok := ratInt64(cashPerShare)
		if !ok || value <= 0 {
			return nil, fmt.Errorf("%w: cash ratio is below fixed-point precision", ErrMalformedResponse)
		}
		actions = append(actions, market.CorporateAction{
			ID:           fmt.Sprintf("eastmoney:%s:%s:cash:%d", id.String(), exDate.Format("2006-01-02"), value),
			Instrument:   id,
			ExDate:       exDate,
			Kind:         market.CashDividend,
			CashPerShare: market.Money(value),
			Version:      0,
		})
	}
	shares := new(big.Rat).Add(bonus, transfer)
	if shares.Sign() > 0 {
		ratio := new(big.Rat).Quo(new(big.Rat).Add(big.NewRat(10, 1), shares), big.NewRat(10, 1))
		numerator, denominator, ok := ratNumeratorDenominator(ratio)
		if !ok || numerator <= denominator {
			return nil, fmt.Errorf("%w: invalid share ratio", ErrMalformedResponse)
		}
		actions = append(actions, market.CorporateAction{
			ID:               fmt.Sprintf("eastmoney:%s:%s:share:%d/%d", id.String(), exDate.Format("2006-01-02"), numerator, denominator),
			Instrument:       id,
			ExDate:           exDate,
			Kind:             market.ShareDistribution,
			ShareNumerator:   numerator,
			ShareDenominator: denominator,
			Version:          0,
		})
	}
	return actions, nil
}

func validateEastmoneyBarRequest(id market.InstrumentID, tf market.Timeframe, from, to time.Time) error {
	if err := id.Validate(); err != nil {
		return fmt.Errorf("%w: invalid instrument: %v", ErrMalformedResponse, err)
	}
	if !tf.Valid() || from.IsZero() || to.IsZero() || from.After(to) {
		return fmt.Errorf("%w: invalid bar request", ErrMalformedResponse)
	}
	return nil
}

func eastmoneySecurityID(id market.InstrumentID) (string, error) {
	switch id.Exchange {
	case market.SSE:
		return "1." + id.Code, nil
	case market.SZSE, market.BSE:
		return "0." + id.Code, nil
	default:
		return "", fmt.Errorf("%w: unsupported exchange", ErrMalformedResponse)
	}
}

func eastmoneyKlinePeriod(tf market.Timeframe) string {
	switch tf {
	case market.Day:
		return "101"
	case market.Week:
		return "102"
	case market.Month:
		return "103"
	default:
		return ""
	}
}

func filterEastmoneyRange(bars []market.Bar, factors []market.AdjustmentFactor, from, to time.Time) ([]market.Bar, []market.AdjustmentFactor, error) {
	filtered := make([]market.Bar, 0, len(bars))
	for _, bar := range bars {
		if (bar.CloseTime.Equal(from) || bar.CloseTime.After(from)) && (bar.CloseTime.Equal(to) || bar.CloseTime.Before(to)) {
			filtered = append(filtered, bar)
		}
	}
	if len(filtered) != len(bars) {
		return nil, nil, fmt.Errorf("%w: response contains bars outside requested range", ErrIncompleteData)
	}
	return filtered, factors, nil
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, value := range values {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}

func parseEastmoneyScaled(value string, scale int64) (int64, error) {
	ratio, err := parseEastmoneyDecimal(value)
	if err != nil {
		return 0, err
	}
	scaled, ok := ratInt64(new(big.Rat).Mul(ratio, big.NewRat(scale, 1)))
	if !ok {
		return 0, errors.New("decimal exceeds fixed-point precision")
	}
	return scaled, nil
}

func parseEastmoneyDecimal(value string) (*big.Rat, error) {
	if !decimalPattern.MatchString(value) {
		return nil, errors.New("not a finite decimal")
	}
	ratio, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return ratio, nil
}

func rawJSONDecimal(value json.RawMessage) (*big.Rat, error) {
	if len(value) == 0 || string(value) == "null" {
		return nil, errors.New("missing decimal")
	}
	var text string
	if value[0] == '"' {
		if err := json.Unmarshal(value, &text); err != nil {
			return nil, err
		}
	} else {
		text = string(value)
	}
	return parseEastmoneyDecimal(strings.TrimSpace(text))
}

func rawJSONString(value json.RawMessage) (string, error) {
	if len(value) == 0 || string(value) == "null" {
		return "", errors.New("missing string")
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func parseEastmoneyActionDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		value = value[:len("2006-01-02")]
	}
	return time.ParseInLocation("2006-01-02", value, time.UTC)
}

func ratInt64(value *big.Rat) (int64, bool) {
	if !value.IsInt() || !value.Num().IsInt64() {
		return 0, false
	}
	return value.Num().Int64(), true
}

func ratNumeratorDenominator(value *big.Rat) (int64, int64, bool) {
	if !value.Num().IsInt64() || !value.Denom().IsInt64() {
		return 0, 0, false
	}
	return value.Num().Int64(), value.Denom().Int64(), true
}

func roundPositiveRat(value *big.Rat) *big.Int {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value.Num(), value.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(value.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func classifyEastmoneyRequestError(err error) error {
	if errors.Is(err, context.Canceled) {
		return upstreamError(ErrRequestCanceled, err, 0, "")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return upstreamError(ErrUpstreamTimeout, err, 0, "")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return upstreamError(ErrUpstreamTimeout, err, 0, "")
	}
	return upstreamError(ErrUpstream, err, 0, "")
}

func upstreamError(kind, cause error, statusCode int, retryAfter string) error {
	duration, ok := parseRetryAfter(retryAfter, time.Now())
	return &UpstreamError{Kind: kind, Cause: cause, StatusCode: statusCode, RetryAfter: duration, HasRetryAfter: ok}
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return when.Sub(now), true
}
