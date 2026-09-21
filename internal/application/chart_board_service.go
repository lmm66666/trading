package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"trading/internal/port"
)

// ErrBoardsFull reports that the chart board list already holds the maximum.
var ErrBoardsFull = errors.New("chart boards are full")

// ErrLastBoard reports that the last remaining chart board cannot be deleted.
var ErrLastBoard = errors.New("last chart board cannot be deleted")

const (
	maxChartBoardNameRunes  = 40
	minChartBoardVisibleBar = 10
	maxChartBoardVisibleBar = 400
	maxChartBoardPanes      = 18
	maxChartBoardPaneWeight = 10000
)

// 默认标的只接受股票现货（6 位数字代码），与 market.InstrumentID 的领域校验同口径；
// 期货主力连续仅用于 comparison，不允许作为看板默认标的。
var chartBoardSymbolPattern = regexp.MustCompile(`^(SSE|SZSE|BSE):[0-9]{6}$`)

var chartBoardComparisons = map[string]bool{
	"INE:SC.MAIN":  true,
	"SHFE:FU.MAIN": true,
	"INE:LU.MAIN":  true,
	"SHFE:AU.MAIN": true,
	"SHFE:AG.MAIN": true,
	"DCE:J.MAIN":   true,
	"DCE:JM.MAIN":  true,
	"CZCE:ZC.MAIN": true,
}

// ChartBoardConfig mirrors the frontend board config DTO; JSON field names
// are part of the storage format and must stay identical.
type ChartBoardConfig struct {
	DefaultSymbol *string            `json:"defaultSymbol"`
	Timeframe     string             `json:"timeframe"`
	PriceView     string             `json:"priceView"`
	Indicators    []IndicatorRequest `json:"indicators"`
	Comparison    *string            `json:"comparison"`
	PaneWeights   map[string]float64 `json:"paneWeights"`
	VisibleBars   int                `json:"visibleBars"`
}

// ChartBoardService orchestrates the single-user chart board use cases: the
// write path is the only entry into storage, so every config is strictly
// parsed, validated, and stored as normalized JSON text.
type ChartBoardService struct {
	store port.ChartBoardStore
}

func NewChartBoardService(store port.ChartBoardStore) (*ChartBoardService, error) {
	if store == nil {
		return nil, invalidRequest("chart board dependencies are required")
	}
	return &ChartBoardService{store: store}, nil
}

func (s *ChartBoardService) List(ctx context.Context) (port.ChartBoardState, error) {
	return s.store.List(ctx)
}

func (s *ChartBoardService) Create(ctx context.Context, name string, config json.RawMessage) (port.ChartBoardState, error) {
	normalizedName, err := normalizeChartBoardName(name)
	if err != nil {
		return port.ChartBoardState{}, err
	}
	normalizedConfig, err := normalizeChartBoardConfig(config)
	if err != nil {
		return port.ChartBoardState{}, err
	}
	if err := ctx.Err(); err != nil {
		return port.ChartBoardState{}, err
	}
	state, err := s.store.List(ctx)
	if err != nil {
		return port.ChartBoardState{}, err
	}
	if len(state.Boards) >= port.MaxChartBoards {
		return port.ChartBoardState{}, ErrBoardsFull
	}
	return s.store.Create(ctx, normalizedName, normalizedConfig)
}

func (s *ChartBoardService) Update(ctx context.Context, id uint64, name *string, config json.RawMessage) (port.ChartBoardState, error) {
	if name == nil && config == nil {
		return port.ChartBoardState{}, invalidRequest("chart board update requires name or config")
	}
	var nextName *string
	if name != nil {
		normalized, err := normalizeChartBoardName(*name)
		if err != nil {
			return port.ChartBoardState{}, err
		}
		nextName = &normalized
	}
	var nextConfig *string
	if config != nil {
		normalized, err := normalizeChartBoardConfig(config)
		if err != nil {
			return port.ChartBoardState{}, err
		}
		nextConfig = &normalized
	}
	if err := ctx.Err(); err != nil {
		return port.ChartBoardState{}, err
	}
	return s.store.Update(ctx, id, nextName, nextConfig)
}

func (s *ChartBoardService) Activate(ctx context.Context, id uint64) (port.ChartBoardState, error) {
	if err := ctx.Err(); err != nil {
		return port.ChartBoardState{}, err
	}
	return s.store.Activate(ctx, id)
}

func (s *ChartBoardService) Delete(ctx context.Context, id uint64) (port.ChartBoardState, error) {
	if err := ctx.Err(); err != nil {
		return port.ChartBoardState{}, err
	}
	state, err := s.store.List(ctx)
	if err != nil {
		return port.ChartBoardState{}, err
	}
	if len(state.Boards) <= 1 {
		return port.ChartBoardState{}, ErrLastBoard
	}
	return s.store.Delete(ctx, id)
}

func normalizeChartBoardName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > maxChartBoardNameRunes {
		return "", invalidRequest("chart board name is invalid")
	}
	return trimmed, nil
}

// normalizeChartBoardConfig strictly parses (unknown fields and trailing
// JSON rejected, recursively into nested indicator objects), validates, and
// re-marshals the config so storage only ever holds canonical JSON text.
func normalizeChartBoardConfig(raw json.RawMessage) (string, error) {
	var config ChartBoardConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return "", invalidRequest("chart board config is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", invalidRequest("chart board config is invalid")
	}
	if err := validateChartBoardConfig(&config); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return "", invalidRequest("chart board config is invalid")
	}
	return string(normalized), nil
}

func validateChartBoardConfig(config *ChartBoardConfig) error {
	if config.DefaultSymbol != nil && !chartBoardSymbolPattern.MatchString(*config.DefaultSymbol) {
		return invalidRequest("chart board default symbol is invalid")
	}
	if config.Timeframe != "DAY" && config.Timeframe != "WEEK" {
		return invalidRequest("chart board timeframe is invalid")
	}
	if config.PriceView != "RAW" && config.PriceView != "QFQ" {
		return invalidRequest("chart board price view is invalid")
	}
	if config.Indicators == nil {
		return invalidRequest("chart board indicators must be an array")
	}
	if len(config.Indicators) > MaxChartIndicators {
		return invalidRequest("chart board indicators exceed bounds")
	}
	seen := make(map[string]bool, len(config.Indicators))
	for _, indicator := range config.Indicators {
		if err := validateIndicatorRequest(indicator); err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%d/%d/%d/%d/%d/%d/%d", indicator.Kind, indicator.Period, indicator.Fast, indicator.Slow, indicator.Signal, indicator.Smooth, indicator.Regime, indicator.Lag)
		if seen[key] {
			return invalidRequest("duplicate chart board indicator")
		}
		seen[key] = true
	}
	if config.Comparison != nil && !chartBoardComparisons[*config.Comparison] {
		return invalidRequest("chart board comparison is unsupported")
	}
	if config.VisibleBars < minChartBoardVisibleBar || config.VisibleBars > maxChartBoardVisibleBar {
		return invalidRequest("chart board visible bars are out of range")
	}
	if config.PaneWeights == nil {
		return invalidRequest("chart board pane weights must be an object")
	}
	if len(config.PaneWeights) > maxChartBoardPanes {
		return invalidRequest("chart board pane weights exceed bounds")
	}
	for _, weight := range config.PaneWeights {
		if weight <= 0 || weight > maxChartBoardPaneWeight || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return invalidRequest("chart board pane weight is invalid")
		}
	}
	return nil
}
