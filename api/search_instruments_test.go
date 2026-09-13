package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"trading/internal/application"
	"trading/internal/market"
	"trading/internal/port"
)

type apiInstrumentQueries struct {
	query application.InstrumentSearchQuery
	items []port.InstrumentSummary
	err   error
}

func (s *apiInstrumentQueries) Search(_ context.Context, query application.InstrumentSearchQuery) ([]port.InstrumentSummary, error) {
	s.query = query
	return s.items, s.err
}

func (s *apiInstrumentQueries) Get(_ context.Context, id market.InstrumentID) (port.InstrumentSummary, error) {
	return port.InstrumentSummary{ID: id, Active: true}, s.err
}

func TestSearchInstrumentsMapsQueryAndMetadata(t *testing.T) {
	f := newKernelFixture(t)
	queries := &apiInstrumentQueries{items: []port.InstrumentSummary{{ID: market.InstrumentID{Exchange: market.SZSE, Code: "002415"}, Name: "海康威视", Board: "MAIN", Active: true, LotSize: 100}}}
	f.services.InstrumentCatalog = queries
	f.router = NewRouter(nil, nil, nil, nil, nil, f.services)

	w := kernelRequest(t, f, "GET", "/api/v1/instruments?q=%E6%B5%B7%E5%BA%B7&exchange=SZSE&limit=10", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, application.InstrumentSearchQuery{Query: "海康", Exchange: market.SZSE, Limit: 10}, queries.query)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[{"instrument":"SZSE:002415","code":"002415","name":"海康威视","exchange":"SZSE","board":"MAIN","lot_size":100}]}}`, w.Body.String())
}

func TestSearchInstrumentsRejectsMissingOrRepeatedParameters(t *testing.T) {
	for _, path := range []string{
		"/api/v1/instruments",
		"/api/v1/instruments?q=600&q=601",
		"/api/v1/instruments?q=600&exchange=NASDAQ",
		"/api/v1/instruments?q=600&limit=0",
	} {
		f := newKernelFixture(t)
		f.services.InstrumentCatalog = &apiInstrumentQueries{}
		f.router = NewRouter(nil, nil, nil, nil, nil, f.services)
		w := kernelRequest(t, f, "GET", path, "")
		require.Equal(t, 400, w.Code, path+": "+w.Body.String())
		require.Contains(t, w.Body.String(), "INVALID_REQUEST")
	}
}
