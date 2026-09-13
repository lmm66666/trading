package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"time"
	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
)

// This adapter reads only the stable request fields required for persistence;
// it deliberately does not import application DTOs into infrastructure.
type backtestInputs struct {
	Instrument market.InstrumentID `json:"instrument"`
	Parameters json.RawMessage     `json:"parameters"`
	Config     json.RawMessage     `json:"config"`
}

func decodeBacktestInputs(raw []byte, result backtest.Result) (backtestInputs, error) {
	var input backtestInputs
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, invalid("invalid backtest request JSON")
	}
	accept := func(id market.InstrumentID) error {
		if id == (market.InstrumentID{}) {
			return nil
		}
		if err := id.Validate(); err != nil {
			return invalid("invalid result instrument")
		}
		if input.Instrument == (market.InstrumentID{}) {
			input.Instrument = id
		}
		if input.Instrument != id {
			return invalid("result instrument differs from request")
		}
		return nil
	}
	for _, o := range result.Orders {
		if err := accept(o.Instrument); err != nil {
			return input, err
		}
	}
	for _, f := range result.Fills {
		if err := accept(f.Instrument); err != nil {
			return input, err
		}
	}
	if err := input.Instrument.Validate(); err != nil {
		return input, invalid("backtest instrument is required")
	}
	if len(input.Parameters) == 0 {
		input.Parameters = json.RawMessage(`{}`)
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{}`)
	}
	return input, nil
}
func validateBacktestResult(result backtest.Result) error {
	if _, err := json.Marshal(result.Summary); err != nil || result.Summary.ClosedTrades < 0 {
		return invalid("invalid backtest summary")
	}
	orders := map[string]backtest.Order{}
	for _, o := range result.Orders {
		if o.ID == "" || o.Quantity < 0 || (o.Side != backtest.Buy && o.Side != backtest.Sell) || o.FinalReason > backtest.UnfilledInvalidAccountTransition || o.CreatedAt.IsZero() || o.CreatedAt.Location() != time.UTC || (!o.AttemptedAt.IsZero() && o.AttemptedAt.Location() != time.UTC) {
			return invalid("invalid backtest order")
		}
		if _, ok := orders[o.ID]; ok {
			return invalid("duplicate backtest order")
		}
		orders[o.ID] = o
	}
	fills := map[string]bool{}
	for _, f := range result.Fills {
		order, ok := orders[f.OrderID]
		if !ok || order.Side != f.Side || f.ID == "" || fills[f.ID] || f.Quantity <= 0 || f.Price <= 0 || f.Gross < 0 || f.Commission < 0 || f.StampDuty < 0 || f.TransferFee < 0 || f.Time.IsZero() || f.Time.Location() != time.UTC {
			return invalid("invalid backtest fill")
		}
		fills[f.ID] = true
	}
	var previous time.Time
	for _, p := range result.Equity {
		if p.Time.IsZero() || p.Time.Location() != time.UTC || (!previous.IsZero() && !p.Time.After(previous)) {
			return invalid("invalid equity point time")
		}
		previous = p.Time
	}
	return nil
}
func insertBacktest(tx *gorm.DB, run ComputeRunModel, result backtest.Result) error {
	if err := validateBacktestResult(result); err != nil {
		return err
	}
	input, err := decodeBacktestInputs(run.RequestJSON, result)
	if err != nil {
		return err
	}
	var instrument InstrumentModel
	if err := tx.Where("exchange = ? AND code = ?", string(input.Instrument.Exchange), input.Instrument.Code).Take(&instrument).Error; err != nil {
		return err
	}
	summary := result.Summary
	model := BacktestRunModel{RunID: run.RunID, InstrumentID: instrument.ID, StrategyID: run.StrategyID, StrategyVersion: run.StrategyVersion, EngineVersion: run.EngineVersion, DataVersion: run.DataVersion, ParametersJSON: input.Parameters, ConfigJSON: input.Config, TotalReturn: summary.TotalReturn, AnnualizedReturn: summary.AnnualizedReturn, MaximumDrawdown: summary.MaximumDrawdown, ClosedTrades: uint64(summary.ClosedTrades), WinRate: summary.WinRate, ProfitFactor: summary.ProfitFactor, AverageHoldingBars: summary.AverageHoldingBars, HasOpenPosition: summary.HasOpenPosition}
	if err := tx.Create(&model).Error; err != nil {
		return err
	}
	sequences := make(map[string]uint64, len(result.Orders))
	if err := batches(len(result.Orders), func(start, end int) error {
		rows := make([]BacktestOrderModel, 0, end-start)
		for i := start; i < end; i++ {
			o := result.Orders[i]
			sequences[o.ID] = uint64(i + 1)
			var attempted *time.Time
			if !o.AttemptedAt.IsZero() {
				at := o.AttemptedAt
				attempted = &at
			}
			rows = append(rows, BacktestOrderModel{RunID: run.RunID, Sequence: uint64(i + 1), OrderID: o.ID, CreatedTime: o.CreatedAt, AttemptedAt: attempted, Reason: o.Reason, Side: strconv.Itoa(int(o.Side)), Quantity: o.Quantity, FinalReason: strconv.Itoa(int(o.FinalReason)), Status: "FINAL"})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		return err
	}
	if err := batches(len(result.Fills), func(start, end int) error {
		rows := make([]BacktestTradeModel, 0, end-start)
		for i := start; i < end; i++ {
			f := result.Fills[i]
			rows = append(rows, BacktestTradeModel{RunID: run.RunID, Sequence: uint64(i + 1), OrderSequence: sequences[f.OrderID], FillID: f.ID, OrderID: f.OrderID, Time: f.Time, Side: strconv.Itoa(int(f.Side)), Quantity: f.Quantity, Price: int64(f.Price), Gross: int64(f.Gross), Commission: int64(f.Commission), StampDuty: int64(f.StampDuty), TransferFee: int64(f.TransferFee)})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		return err
	}
	return batches(len(result.Equity), func(start, end int) error {
		rows := make([]BacktestEquityModel, 0, end-start)
		for i := start; i < end; i++ {
			p := result.Equity[i]
			rows = append(rows, BacktestEquityModel{RunID: run.RunID, Sequence: uint64(i + 1), Time: p.Time, Cash: int64(p.Cash), PositionValue: int64(p.PositionValue), Equity: int64(p.Equity)})
		}
		return tx.Create(&rows).Error
	})
}

type backtestStored struct {
	BacktestRunModel
	Exchange string
	Code     string
}

func publishedBacktest(db *gorm.DB, runID string) (backtestStored, error) {
	var row backtestStored
	if err := port.ValidateIdentity(runID, "run ID", port.MaxRunIDBytes, false); err != nil {
		return row, err
	}
	err := db.Table("t_backtest_runs AS b").Select("b.*, i.exchange, i.code").Joins("JOIN t_compute_runs AS r ON r.run_id = b.run_id AND r.status = 'SUCCEEDED'").Joins("JOIN t_instruments AS i ON i.id = b.instrument_id").Where("b.run_id = ?", runID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = port.ErrRunNotFound
	}
	return row, err
}
func (s *RunStore) BacktestResult(ctx context.Context, runID string) (backtest.Summary, error) {
	row, err := publishedBacktest(s.db.WithContext(ctx), runID)
	if err != nil {
		return backtest.Summary{}, err
	}
	return backtest.Summary{TotalReturn: row.TotalReturn, AnnualizedReturn: row.AnnualizedReturn, MaximumDrawdown: row.MaximumDrawdown, ClosedTrades: int(row.ClosedTrades), WinRate: row.WinRate, ProfitFactor: row.ProfitFactor, AverageHoldingBars: row.AverageHoldingBars, HasOpenPosition: row.HasOpenPosition}, nil
}
func resultPage[M, T any](ctx context.Context, s *RunStore, runID string, page port.PageRequest, convert func(M, market.InstrumentID) (T, error)) (port.Page[T], error) {
	if err := page.Validate(); err != nil {
		return port.Page[T]{}, err
	}
	row, err := publishedBacktest(s.db.WithContext(ctx), runID)
	if err != nil {
		return port.Page[T]{}, err
	}
	instrument := market.InstrumentID{Exchange: market.Exchange(row.Exchange), Code: row.Code}
	var models []M
	if err := s.db.WithContext(ctx).Where("run_id = ? AND sequence > ?", runID, page.AfterSequence).Order("sequence").Limit(page.Limit + 1).Find(&models).Error; err != nil {
		return port.Page[T]{}, err
	}
	result := port.Page[T]{Items: make([]T, 0, min(len(models), page.Limit))}
	if len(models) > page.Limit {
		models = models[:page.Limit]
		var sequence int64
		switch m := any(models[len(models)-1]).(type) {
		case BacktestOrderModel:
			sequence = int64(m.Sequence)
		case BacktestTradeModel:
			sequence = int64(m.Sequence)
		case BacktestEquityModel:
			sequence = int64(m.Sequence)
		}
		result.NextSequence = &sequence
	}
	for _, model := range models {
		item, err := convert(model, instrument)
		if err != nil {
			return port.Page[T]{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
func (s *RunStore) Orders(ctx context.Context, runID string, page port.PageRequest) (port.Page[backtest.Order], error) {
	return resultPage(ctx, s, runID, page, func(row BacktestOrderModel, id market.InstrumentID) (backtest.Order, error) {
		side, err := strconv.ParseUint(row.Side, 10, 8)
		if err != nil || (side != uint64(backtest.Buy) && side != uint64(backtest.Sell)) {
			return backtest.Order{}, invalid("invalid stored order side")
		}
		reason, err := strconv.ParseUint(row.FinalReason, 10, 8)
		if err != nil || reason > uint64(backtest.UnfilledInvalidAccountTransition) {
			return backtest.Order{}, invalid("invalid stored order outcome")
		}
		order := backtest.Order{ID: row.OrderID, Instrument: id, Side: backtest.Side(side), Quantity: row.Quantity, CreatedAt: row.CreatedTime.UTC(), Reason: row.Reason, FinalReason: backtest.OrderFinalReason(reason)}
		if row.AttemptedAt != nil {
			order.AttemptedAt = row.AttemptedAt.UTC()
		}
		return order, nil
	})
}
func (s *RunStore) Trades(ctx context.Context, runID string, page port.PageRequest) (port.Page[backtest.Fill], error) {
	return resultPage(ctx, s, runID, page, func(row BacktestTradeModel, id market.InstrumentID) (backtest.Fill, error) {
		side, err := strconv.ParseUint(row.Side, 10, 8)
		if err != nil || (side != uint64(backtest.Buy) && side != uint64(backtest.Sell)) {
			return backtest.Fill{}, invalid("invalid stored fill side")
		}
		return backtest.Fill{ID: row.FillID, OrderID: row.OrderID, Instrument: id, Side: backtest.Side(side), Time: row.Time.UTC(), Quantity: row.Quantity, Price: market.Price(row.Price), Gross: market.Money(row.Gross), Commission: market.Money(row.Commission), StampDuty: market.Money(row.StampDuty), TransferFee: market.Money(row.TransferFee)}, nil
	})
}
func (s *RunStore) Equity(ctx context.Context, runID string, page port.PageRequest) (port.Page[backtest.EquityPoint], error) {
	return resultPage(ctx, s, runID, page, func(row BacktestEquityModel, _ market.InstrumentID) (backtest.EquityPoint, error) {
		return backtest.EquityPoint{Time: row.Time.UTC(), Equity: market.Money(row.Equity), Cash: market.Money(row.Cash), PositionValue: market.Money(row.PositionValue)}, nil
	})
}
