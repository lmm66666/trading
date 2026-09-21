package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/port"
)

type chartBoardStoreFake struct {
	state                         port.ChartBoardState
	nextID                        uint64
	createName, createConfig      string
	updateID                      uint64
	updateName, updateConfig      *string
	activateID, deleteID          uint64
	created, updated, deleted     int
	listErr, createErr, updateErr error
	activateErr, deleteErr        error
}

func (s *chartBoardStoreFake) List(context.Context) (port.ChartBoardState, error) {
	return s.state, s.listErr
}

func (s *chartBoardStoreFake) Create(_ context.Context, name, config string) (port.ChartBoardState, error) {
	s.created++
	s.createName, s.createConfig = name, config
	if s.createErr != nil {
		return port.ChartBoardState{}, s.createErr
	}
	s.nextID++
	s.state.Boards = append(s.state.Boards, port.ChartBoard{ID: s.nextID, Name: name, Config: config})
	s.state.ActiveID = s.nextID
	return s.state, nil
}

func (s *chartBoardStoreFake) Update(_ context.Context, id uint64, name *string, config *string) (port.ChartBoardState, error) {
	s.updated++
	s.updateID, s.updateName, s.updateConfig = id, name, config
	if s.updateErr != nil {
		return port.ChartBoardState{}, s.updateErr
	}
	for index := range s.state.Boards {
		if s.state.Boards[index].ID != id {
			continue
		}
		if name != nil {
			s.state.Boards[index].Name = *name
		}
		if config != nil {
			s.state.Boards[index].Config = *config
		}
	}
	return s.state, nil
}

func (s *chartBoardStoreFake) Activate(_ context.Context, id uint64) (port.ChartBoardState, error) {
	s.activateID = id
	if s.activateErr != nil {
		return port.ChartBoardState{}, s.activateErr
	}
	s.state.ActiveID = id
	return s.state, nil
}

func (s *chartBoardStoreFake) Delete(_ context.Context, id uint64) (port.ChartBoardState, error) {
	s.deleted++
	s.deleteID = id
	if s.deleteErr != nil {
		return port.ChartBoardState{}, s.deleteErr
	}
	boards := make([]port.ChartBoard, 0, len(s.state.Boards))
	for _, board := range s.state.Boards {
		if board.ID != id {
			boards = append(boards, board)
		}
	}
	s.state.Boards = boards
	if s.state.ActiveID == id && len(boards) > 0 {
		s.state.ActiveID = boards[0].ID
	}
	return s.state, nil
}

func newChartBoardFixture(t *testing.T) (*ChartBoardService, *chartBoardStoreFake) {
	t.Helper()
	store := &chartBoardStoreFake{}
	service, err := NewChartBoardService(store)
	require.NoError(t, err)
	return service, store
}

func pointerOf(value string) *string { return &value }

func validBoardConfigJSON() string {
	return `{"defaultSymbol":"SSE:600938","timeframe":"DAY","priceView":"QFQ","indicators":[{"kind":"SMA","period":5},{"kind":"MACD","fast":12,"slow":26,"signal":9}],"comparison":"INE:SC.MAIN","paneWeights":{"price":3,"volume":1},"visibleBars":120}`
}

func normalizedBoardConfigJSON() string {
	return `{"defaultSymbol":"SSE:600938","timeframe":"DAY","priceView":"QFQ","indicators":[{"kind":"SMA","period":5},{"kind":"MACD","fast":12,"slow":26,"signal":9}],"comparison":"INE:SC.MAIN","paneWeights":{"price":3,"volume":1},"visibleBars":120}`
}

func TestNewChartBoardServiceRequiresStore(t *testing.T) {
	_, err := NewChartBoardService(nil)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestChartBoardCreateValidatesConfig(t *testing.T) {
	replacement := func(mutate func(config map[string]any)) string {
		config := map[string]any{
			"defaultSymbol": "SSE:600938",
			"timeframe":     "DAY",
			"priceView":     "QFQ",
			"indicators":    []any{map[string]any{"kind": "SMA", "period": 5}},
			"comparison":    nil,
			"paneWeights":   map[string]any{"price": 3},
			"visibleBars":   120,
		}
		mutate(config)
		raw, err := json.Marshal(config)
		require.NoError(t, err)
		return string(raw)
	}
	for name, body := range map[string]string{
		"bad default symbol":   replacement(func(c map[string]any) { c["defaultSymbol"] = "evil" }),
		"unsupported exchange": replacement(func(c map[string]any) { c["defaultSymbol"] = "NASDAQ:AAPL" }),
		"bad timeframe":        replacement(func(c map[string]any) { c["timeframe"] = "MONTH" }),
		"bad price view":       replacement(func(c map[string]any) { c["priceView"] = "HFQ" }),
		"null indicators":      replacement(func(c map[string]any) { c["indicators"] = nil }),
		"absent indicators":    `{"defaultSymbol":null,"timeframe":"DAY","priceView":"QFQ","comparison":null,"paneWeights":{},"visibleBars":120}`,
		"too many indicators": replacement(func(c map[string]any) {
			indicators := make([]any, MaxChartIndicators+1)
			for index := range indicators {
				indicators[index] = map[string]any{"kind": "SMA", "period": index + 1}
			}
			c["indicators"] = indicators
		}),
		"invalid indicator": replacement(func(c map[string]any) { c["indicators"] = []any{map[string]any{"kind": "SMA", "period": 0}} }),
		"invalid MACD": replacement(func(c map[string]any) {
			c["indicators"] = []any{map[string]any{"kind": "MACD", "fast": 26, "slow": 12, "signal": 9}}
		}),
		"duplicate indicator": replacement(func(c map[string]any) {
			c["indicators"] = []any{map[string]any{"kind": "SMA", "period": 5}, map[string]any{"kind": "SMA", "period": 5}}
		}),
		"unsupported comparison": replacement(func(c map[string]any) { c["comparison"] = "WTI" }),
		"null pane weights":      replacement(func(c map[string]any) { c["paneWeights"] = nil }),
		"too many panes": replacement(func(c map[string]any) {
			weights := map[string]any{}
			for index := 0; index <= maxChartBoardPanes; index++ {
				weights[fmt.Sprintf("pane%d", index)] = 1
			}
			c["paneWeights"] = weights
		}),
		"zero pane weight":      replacement(func(c map[string]any) { c["paneWeights"] = map[string]any{"price": 0} }),
		"oversized pane weight": replacement(func(c map[string]any) { c["paneWeights"] = map[string]any{"price": 10001} }),
		"visible bars too few":  replacement(func(c map[string]any) { c["visibleBars"] = 9 }),
		"visible bars too many": replacement(func(c map[string]any) { c["visibleBars"] = 401 }),
		"unknown config field":  `{"defaultSymbol":null,"timeframe":"DAY","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120,"extra":1}`,
		"trailing config JSON":  validBoardConfigJSON() + ` {}`,
		"broken config JSON":    `{`,
	} {
		t.Run(name, func(t *testing.T) {
			service, store := newChartBoardFixture(t)
			_, err := service.Create(context.Background(), "看板", json.RawMessage(body))
			require.ErrorIs(t, err, ErrInvalidRequest)
			require.Zero(t, store.created)
		})
	}
}

func TestChartBoardCreateValidatesName(t *testing.T) {
	for _, name := range []string{"", "   ", strings.Repeat("名", maxChartBoardNameRunes+1)} {
		service, store := newChartBoardFixture(t)
		_, err := service.Create(context.Background(), name, json.RawMessage(validBoardConfigJSON()))
		require.ErrorIs(t, err, ErrInvalidRequest)
		require.Zero(t, store.created)
	}
}

func TestChartBoardCreateNormalizesAndReturnsState(t *testing.T) {
	service, store := newChartBoardFixture(t)
	// 输入键序打乱、带空白，输出必须是规范化 JSON。
	shuffled := ` { "paneWeights": {"volume":1,"price":3}, "visibleBars": 120, "comparison": "INE:SC.MAIN",
		"indicators": [ {"period": 5, "kind": "SMA"}, {"kind":"MACD","fast":12,"slow":26,"signal":9} ],
		"priceView": "QFQ", "timeframe": "DAY", "defaultSymbol": "SSE:600938" } `
	state, err := service.Create(context.Background(), "  油价看板  ", json.RawMessage(shuffled))
	require.NoError(t, err)
	require.Equal(t, "油价看板", store.createName)
	require.JSONEq(t, normalizedBoardConfigJSON(), store.createConfig)
	require.Equal(t, store.state, state)
	require.Len(t, state.Boards, 1)
	require.Equal(t, state.ActiveID, state.Boards[0].ID)
}

func TestChartBoardCreateRejectsWhenFull(t *testing.T) {
	service, store := newChartBoardFixture(t)
	store.state = port.ChartBoardState{Boards: make([]port.ChartBoard, port.MaxChartBoards)}
	for index := range store.state.Boards {
		store.state.Boards[index] = port.ChartBoard{ID: uint64(index + 1), Name: "看板", Config: normalizedBoardConfigJSON()}
	}
	_, err := service.Create(context.Background(), "新看板", json.RawMessage(validBoardConfigJSON()))
	require.ErrorIs(t, err, ErrBoardsFull)
	require.Zero(t, store.created)
}

func TestChartBoardUpdateVariants(t *testing.T) {
	service, store := newChartBoardFixture(t)
	_, err := service.Update(context.Background(), 1, nil, nil)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Zero(t, store.updated)

	state, err := service.Update(context.Background(), 7, pointerOf("  周线  "), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(7), store.updateID)
	require.Equal(t, "周线", *store.updateName)
	require.Nil(t, store.updateConfig)
	require.Equal(t, store.state, state)

	state, err = service.Update(context.Background(), 7, nil, json.RawMessage(validBoardConfigJSON()))
	require.NoError(t, err)
	require.Nil(t, store.updateName)
	require.NotNil(t, store.updateConfig)
	require.JSONEq(t, normalizedBoardConfigJSON(), *store.updateConfig)
	require.Equal(t, store.state, state)

	name := "双改"
	state, err = service.Update(context.Background(), 7, &name, json.RawMessage(validBoardConfigJSON()))
	require.NoError(t, err)
	require.Equal(t, "双改", *store.updateName)
	require.NotNil(t, store.updateConfig)
	require.Equal(t, store.state, state)

	store.updateErr = fmt.Errorf("%w: board 7", port.ErrChartBoardNotFound)
	_, err = service.Update(context.Background(), 7, &name, nil)
	require.ErrorIs(t, err, port.ErrChartBoardNotFound)
}

func TestChartBoardDeleteGuardsLastBoard(t *testing.T) {
	service, store := newChartBoardFixture(t)
	store.state = port.ChartBoardState{Boards: []port.ChartBoard{{ID: 1, Name: "唯一", Config: normalizedBoardConfigJSON()}}, ActiveID: 1}
	_, err := service.Delete(context.Background(), 1)
	require.ErrorIs(t, err, ErrLastBoard)
	require.Zero(t, store.deleted)

	store.state.Boards = append(store.state.Boards, port.ChartBoard{ID: 2, Name: "第二", Config: normalizedBoardConfigJSON()})
	state, err := service.Delete(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), store.deleteID)
	require.Equal(t, store.state, state)
}

func TestChartBoardListAndActivatePropagateStore(t *testing.T) {
	service, store := newChartBoardFixture(t)
	store.listErr = errors.New("store down")
	_, err := service.List(context.Background())
	require.ErrorIs(t, err, store.listErr)
	_, err = service.Delete(context.Background(), 1)
	require.ErrorIs(t, err, store.listErr)

	store.listErr = nil
	store.state = port.ChartBoardState{Boards: []port.ChartBoard{{ID: 3, Name: "看板", Config: normalizedBoardConfigJSON()}}, ActiveID: 3}
	state, err := service.Activate(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), store.activateID)
	require.Equal(t, store.state, state)

	store.activateErr = fmt.Errorf("%w: board 9", port.ErrChartBoardNotFound)
	_, err = service.Activate(context.Background(), 9)
	require.ErrorIs(t, err, port.ErrChartBoardNotFound)
}

func TestChartBoardZSCOREIdentityAndRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"defaultSymbol":null,"timeframe":"DAY","priceView":"RAW","indicators":[{"kind":"ZSCORE","period":126,"smooth":5,"regime":252},{"kind":"ZSCORE","period":126,"smooth":10,"regime":252}],"comparison":null,"paneWeights":{},"visibleBars":120}`)
	normalized, err := normalizeChartBoardConfig(raw)
	require.NoError(t, err)
	var config ChartBoardConfig
	require.NoError(t, json.Unmarshal([]byte(normalized), &config))
	require.Equal(t, 10, config.Indicators[1].Smooth)
	config.Indicators[1] = config.Indicators[0]
	require.ErrorIs(t, validateChartBoardConfig(&config), ErrInvalidRequest)
}

func TestLegacyIndicatorsRejectZSCOREParameters(t *testing.T) {
	for _, request := range []IndicatorRequest{{Kind: IndicatorSMA, Period: 5}, {Kind: IndicatorEMA, Period: 5}, {Kind: IndicatorKDJ, Period: 9}, {Kind: IndicatorMACD, Fast: 12, Slow: 26, Signal: 9}} {
		for _, field := range []string{"smooth", "regime"} {
			next := request
			if field == "smooth" {
				next.Smooth = 5
			} else {
				next.Regime = 252
			}
			require.ErrorIs(t, validateIndicatorRequest(next), ErrInvalidRequest, "%s %s", request.Kind, field)
		}
	}
}
