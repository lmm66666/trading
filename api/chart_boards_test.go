package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/port"
)

type apiChartBoards struct {
	state                         port.ChartBoardState
	createName                    string
	createConfig                  json.RawMessage
	updateID                      uint64
	updateName                    *string
	updateConfig                  json.RawMessage
	activateID, deleteID          uint64
	created, listed               int
	listErr, createErr, updateErr error
	activateErr, deleteErr        error
}

func (s *apiChartBoards) List(context.Context) (port.ChartBoardState, error) {
	s.listed++
	return s.state, s.listErr
}

func (s *apiChartBoards) Create(_ context.Context, name string, config json.RawMessage) (port.ChartBoardState, error) {
	s.created++
	s.createName, s.createConfig = name, config
	return s.state, s.createErr
}

func (s *apiChartBoards) Update(_ context.Context, id uint64, name *string, config json.RawMessage) (port.ChartBoardState, error) {
	s.updateID, s.updateName, s.updateConfig = id, name, config
	return s.state, s.updateErr
}

func (s *apiChartBoards) Activate(_ context.Context, id uint64) (port.ChartBoardState, error) {
	s.activateID = id
	return s.state, s.activateErr
}

func (s *apiChartBoards) Delete(_ context.Context, id uint64) (port.ChartBoardState, error) {
	s.deleteID = id
	return s.state, s.deleteErr
}

func chartBoardFixtureState() port.ChartBoardState {
	return port.ChartBoardState{
		Boards: []port.ChartBoard{
			{ID: 1, Name: "默认看板", Config: `{"defaultSymbol":null,"timeframe":"DAY","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120}`},
			{ID: 2, Name: "油价", Config: `{"defaultSymbol":"SSE:600938","timeframe":"WEEK","priceView":"RAW","indicators":[{"kind":"MACD","fast":12,"slow":26,"signal":9}],"comparison":"INE:SC.MAIN","paneWeights":{"price":3},"visibleBars":200}`},
		},
		ActiveID: 2,
	}
}

func TestListChartBoardsReturnsState(t *testing.T) {
	f := newKernelFixture(t)
	boards := &apiChartBoards{state: chartBoardFixtureState()}
	f.services.ChartBoards = boards
	f.router = NewRouter(f.services)
	w := kernelRequest(t, f, "GET", "/api/v1/chart-boards", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.JSONEq(t, `{"code":0,"message":"success","data":{
		"boards":[
			{"id":1,"name":"默认看板","config":{"defaultSymbol":null,"timeframe":"DAY","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120}},
			{"id":2,"name":"油价","config":{"defaultSymbol":"SSE:600938","timeframe":"WEEK","priceView":"RAW","indicators":[{"kind":"MACD","fast":12,"slow":26,"signal":9}],"comparison":"INE:SC.MAIN","paneWeights":{"price":3},"visibleBars":200}}
		],
		"active_id":2
	}}`, w.Body.String())
	require.Equal(t, 1, boards.listed)
}

func TestChartBoardRoutesRequireConfiguration(t *testing.T) {
	f := newKernelFixture(t)
	f.router = NewRouter(f.services)
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/api/v1/chart-boards", ""},
		{"POST", "/api/v1/chart-boards", `{"name":"看板","config":{}}`},
		{"PUT", "/api/v1/chart-boards/1", `{"name":"看板"}`},
		{"POST", "/api/v1/chart-boards/1/activate", ""},
		{"DELETE", "/api/v1/chart-boards/1", ""},
	} {
		w := kernelRequest(t, f, route.method, route.path, route.body)
		require.Equal(t, 500, w.Code, route.path)
		require.Contains(t, w.Body.String(), "internal server error")
	}
}

func TestCreateChartBoardPassesBodyAndMapsErrors(t *testing.T) {
	f := newKernelFixture(t)
	boards := &apiChartBoards{state: chartBoardFixtureState()}
	f.services.ChartBoards = boards
	f.router = NewRouter(f.services)
	body := `{"name":"新看板","config":{"defaultSymbol":"SSE:600938","timeframe":"DAY","priceView":"QFQ","indicators":[{"kind":"SMA","period":5}],"comparison":null,"paneWeights":{"price":3},"visibleBars":120}}`
	w := kernelRequest(t, f, "POST", "/api/v1/chart-boards", body)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "新看板", boards.createName)
	require.JSONEq(t, `{"defaultSymbol":"SSE:600938","timeframe":"DAY","priceView":"QFQ","indicators":[{"kind":"SMA","period":5}],"comparison":null,"paneWeights":{"price":3},"visibleBars":120}`, string(boards.createConfig))
	require.Equal(t, 1, boards.created)

	for name, tt := range map[string]struct {
		body   string
		err    error
		status int
		msg    string
	}{
		"unknown top field":  {`{"name":"x","config":{},"extra":1}`, nil, 400, "INVALID_REQUEST"},
		"trailing json":      {body + " {}", nil, 400, "INVALID_REQUEST"},
		"broken json":        {`{`, nil, 400, "INVALID_REQUEST"},
		"boards full":        {body, application.ErrBoardsFull, 409, "BOARDS_FULL"},
		"invalid by service": {body, application.ErrInvalidRequest, 400, "INVALID_REQUEST"},
	} {
		t.Run(name, func(t *testing.T) {
			boards.createErr = tt.err
			w := kernelRequest(t, f, "POST", "/api/v1/chart-boards", tt.body)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tt.msg)
		})
	}
}

func TestUpdateChartBoardVariantsAndErrors(t *testing.T) {
	f := newKernelFixture(t)
	boards := &apiChartBoards{state: chartBoardFixtureState()}
	f.services.ChartBoards = boards
	f.router = NewRouter(f.services)

	w := kernelRequest(t, f, "PUT", "/api/v1/chart-boards/12", `{"name":"重命名"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, uint64(12), boards.updateID)
	require.Equal(t, "重命名", *boards.updateName)
	require.Nil(t, boards.updateConfig)

	config := `{"defaultSymbol":null,"timeframe":"WEEK","priceView":"QFQ","indicators":[],"comparison":null,"paneWeights":{},"visibleBars":120}`
	w = kernelRequest(t, f, "PUT", "/api/v1/chart-boards/12", `{"config":`+config+`}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Nil(t, boards.updateName)
	require.NotNil(t, boards.updateConfig)
	require.JSONEq(t, config, string(boards.updateConfig))

	w = kernelRequest(t, f, "PUT", "/api/v1/chart-boards/12", `{"name":"a","config":`+config+`}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "a", *boards.updateName)
	require.NotNil(t, boards.updateConfig)

	for name, tt := range map[string]struct {
		path   string
		body   string
		err    error
		status int
		msg    string
	}{
		"non numeric id":  {"/api/v1/chart-boards/abc", `{"name":"x"}`, nil, 400, "INVALID_REQUEST"},
		"empty body":      {"/api/v1/chart-boards/12", `{}`, application.ErrInvalidRequest, 400, "INVALID_REQUEST"},
		"unknown board":   {"/api/v1/chart-boards/12", `{"name":"x"}`, port.ErrChartBoardNotFound, 404, "NOT_FOUND"},
		"internal parity": {"/api/v1/chart-boards/12", `{"name":"x"}`, internalAPIError, 500, "internal server error"},
	} {
		t.Run(name, func(t *testing.T) {
			boards.updateErr = tt.err
			w := kernelRequest(t, f, "PUT", tt.path, tt.body)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tt.msg)
			if tt.err == internalAPIError {
				require.NotContains(t, w.Body.String(), "secret")
			}
		})
	}
}

func TestActivateAndDeleteChartBoardMapErrors(t *testing.T) {
	f := newKernelFixture(t)
	boards := &apiChartBoards{state: chartBoardFixtureState()}
	f.services.ChartBoards = boards
	f.router = NewRouter(f.services)

	w := kernelRequest(t, f, "POST", "/api/v1/chart-boards/7/activate", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, uint64(7), boards.activateID)

	w = kernelRequest(t, f, "POST", "/api/v1/chart-boards/x/activate", "")
	require.Equal(t, 400, w.Code)
	boards.activateErr = port.ErrChartBoardNotFound
	w = kernelRequest(t, f, "POST", "/api/v1/chart-boards/7/activate", "")
	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "NOT_FOUND")

	w = kernelRequest(t, f, "DELETE", "/api/v1/chart-boards/9", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, uint64(9), boards.deleteID)

	w = kernelRequest(t, f, "DELETE", "/api/v1/chart-boards/9.5", "")
	require.Equal(t, 400, w.Code)
	boards.deleteErr = application.ErrLastBoard
	w = kernelRequest(t, f, "DELETE", "/api/v1/chart-boards/9", "")
	require.Equal(t, 409, w.Code)
	require.Contains(t, w.Body.String(), "LAST_BOARD")
	boards.deleteErr = port.ErrChartBoardNotFound
	w = kernelRequest(t, f, "DELETE", "/api/v1/chart-boards/9", "")
	require.Equal(t, 404, w.Code)
}
