package mysql

import (
	"strings"
	"trading/internal/market"
	"trading/internal/port"
)

type parameterizedQuery struct {
	sql  string
	args []any
}

// At most 15,000 (instrument,timeframe) pairs: 5,000 instruments and three
// timeframes. Two 7,500-branch statements keep each prepared statement below
// MySQL's 65,535-parameter ceiling (six parameters per branch). UNION operands
// each use the single-instrument index and LIMIT; no full-history transfer.
func lookbackQueries(ids []uint64, req port.BatchRequest) []parameterizedQuery {
	if req.LookbackBars == 0 || len(ids) == 0 {
		return nil
	}
	const branchesPerQuery = 7500
	const branch = "(SELECT * FROM t_market_bars WHERE instrument_id = ? AND timeframe = ? AND close_time < ? AND valid_from_version <= ? AND (valid_to_version IS NULL OR valid_to_version > ?) ORDER BY close_time DESC LIMIT ?)"
	timeframes := append([]market.Timeframe{req.PrimaryTimeframe}, req.Auxiliary...)
	var queries []parameterizedQuery
	var parts []string
	var args []any
	flush := func() {
		if len(parts) > 0 {
			queries = append(queries, parameterizedQuery{sql: strings.Join(parts, " UNION ALL "), args: args})
			parts = nil
			args = nil
		}
	}
	for _, id := range ids {
		for _, tf := range timeframes {
			parts = append(parts, branch)
			args = append(args, id, timeframeName(tf), req.From, uint64(req.Version), uint64(req.Version), req.LookbackBars)
			if len(parts) == branchesPerQuery {
				flush()
			}
		}
	}
	flush()
	return queries
}
