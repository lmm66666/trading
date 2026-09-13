package mysql

import (
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
	"trading/internal/market"
)

func TestRevisionDiffKeepsUnchangedAndNeverDeletesOmittedKeys(t *testing.T) {
	b, _, err := canonicalBatch(testBatch(100000))
	require.NoError(t, err)
	bar := b.Bars[market.Day][0]
	row := barModel(41, bar, 4, 1)
	row.ID = 7
	closed, inserted, err := barChanges(41, b, []MarketBarModel{row}, 2)
	require.NoError(t, err)
	require.Empty(t, closed)
	require.Empty(t, inserted)
	b.Bars[market.Day][0].Close = 110000
	closed, inserted, err = barChanges(41, b, []MarketBarModel{row}, 2)
	require.NoError(t, err)
	require.Equal(t, []uint64{7}, closed)
	require.Equal(t, uint32(5), inserted[0].Revision)
	b.Bars = nil
	closed, inserted, err = barChanges(41, b, []MarketBarModel{row}, 2)
	require.NoError(t, err)
	require.Empty(t, closed)
	require.Empty(t, inserted)
	_, _, err = barChanges(41, b, []MarketBarModel{row, row}, 2)
	require.Error(t, err)
	b.Bars = map[market.Timeframe][]market.Bar{market.Day: {bar}}
	b.Bars[market.Day][0].Close = 110000
	row.Revision = math.MaxUint32
	_, _, err = barChanges(41, b, []MarketBarModel{row}, 2)
	require.Error(t, err)
	f := market.AdjustmentFactor{EffectiveTime: bar.OpenTime, Numerator: 1, Denominator: 1}
	fr := AdjustmentFactorModel{BaseModel: BaseModel{ID: 8}, EffectiveTime: f.EffectiveTime, Numerator: 1, Denominator: 1}
	fc, fi, err := factorChanges(41, []market.AdjustmentFactor{f}, []AdjustmentFactorModel{fr}, 2)
	require.NoError(t, err)
	require.Empty(t, fc)
	require.Empty(t, fi)
	_, _, err = factorChanges(41, []market.AdjustmentFactor{f}, []AdjustmentFactorModel{fr, fr}, 2)
	require.Error(t, err)
	a := market.CorporateAction{ID: "event", Instrument: b.Instrument, ExDate: bar.OpenTime, Kind: market.CashDividend, CashPerShare: 100}
	ar := actionModel(41, a, 1)
	ar.ID = 9
	b.Actions = []market.CorporateAction{a}
	ac, ai, err := actionChanges(41, b, []CorporateActionModel{ar}, 2)
	require.NoError(t, err)
	require.Empty(t, ac)
	require.Empty(t, ai)
	_, _, err = actionChanges(41, b, []CorporateActionModel{ar, ar}, 2)
	require.Error(t, err)
}

func TestCanonicalRejectsAmbiguousTimesAndDuplicateEvents(t *testing.T) {
	b := testBatch(100000)
	bar := b.Bars[market.Day][0]
	for _, at := range []time.Time{time.Time{}, bar.OpenTime.In(time.FixedZone("local", 8*3600)), bar.OpenTime.Add(time.Nanosecond), time.Date(999, 1, 1, 0, 0, 0, 0, time.UTC)} {
		require.Error(t, storedTime(at))
	}
	bar.CloseTime = bar.OpenTime.Add(-time.Second)
	require.Error(t, validateStoredBar(bar))
	bar = b.Bars[market.Day][0]
	limit := market.Price(-1)
	bar.LimitDown = &limit
	require.Error(t, validateStoredBar(bar))
	b.Source = strings.Repeat("x", 129)
	_, _, err := canonicalBatch(b)
	require.Error(t, err)
	b = testBatch(100000)
	f := market.AdjustmentFactor{EffectiveTime: bar.OpenTime, Numerator: 1, Denominator: 1}
	b.Factors = []market.AdjustmentFactor{f, f}
	_, _, err = canonicalBatch(b)
	require.Error(t, err)
	b = testBatch(100000)
	a := market.CorporateAction{ID: "event", Instrument: b.Instrument, ExDate: bar.OpenTime, Kind: market.CashDividend}
	b.Actions = []market.CorporateAction{a, a}
	_, _, err = canonicalBatch(b)
	require.Error(t, err)
	b = testBatch(100000)
	b.Bars[market.Day] = nil
	_, _, err = canonicalBatch(b)
	require.Error(t, err)
}

func TestCanonicalRejectsInvalidUTF8BeforeJSONHashing(t *testing.T) {
	b := testBatch(100000)
	b.Source = string([]byte{0xff})
	_, _, err := canonicalBatch(b)
	require.Error(t, err)
	b = testBatch(100000)
	b.Actions = []market.CorporateAction{{ID: string([]byte{0xff}), Instrument: b.Instrument, ExDate: b.Bars[market.Day][0].OpenTime, Kind: market.CashDividend}}
	_, _, err = canonicalBatch(b)
	require.Error(t, err)
}

func TestActionAndStatusRoundTripsValidateStoredValues(t *testing.T) {
	b := testBatch(100000)
	bar := b.Bars[market.Day][0]
	for _, tf := range []market.Timeframe{market.Day, market.Week, market.Month} {
		bar.Timeframe = tf
		bar.Trading = market.Suspended
		row := barModel(41, bar, 1, 1)
		got, err := row.bar(b.Instrument, 2)
		require.NoError(t, err)
		require.Equal(t, market.Suspended, got.Trading)
		require.Equal(t, tf, got.Timeframe)
	}
	row := barModel(41, bar, 1, 1)
	row.TradingStatus = "BAD"
	_, err := row.bar(b.Instrument, 1)
	require.Error(t, err)
	for _, kind := range []market.CorporateActionKind{market.CashDividend, market.ShareDistribution, market.RightsIssue} {
		a := market.CorporateAction{ID: "event", Instrument: b.Instrument, ExDate: bar.OpenTime, Kind: kind}
		if kind == market.ShareDistribution {
			a.ShareNumerator = 2
			a.ShareDenominator = 1
		}
		r := actionModel(41, a, 1)
		got, err := r.action(b.Instrument, 2)
		require.NoError(t, err)
		require.Equal(t, kind, got.Kind)
		require.Equal(t, market.DataVersion(2), got.Version)
	}
	a := market.CorporateAction{ID: "event", Instrument: b.Instrument, ExDate: bar.OpenTime, Kind: market.ShareDistribution}
	require.Error(t, validateAction(a))
	a.Kind = market.CashDividend
	a.ShareNumerator = 1
	require.Error(t, validateAction(a))
	a.ShareNumerator = 0
	a.CashPerShare = -1
	require.Error(t, validateAction(a))
	a.CashPerShare = 0
	a.ID = " "
	require.Error(t, validateAction(a))
}

func TestSchemaDeclaresVersionAndQueryIndexes(t *testing.T) {
	for _, entry := range migrationModels {
		s, err := schema.Parse(entry.model, &sync.Map{}, schema.NamingStrategy{})
		require.NoError(t, err)
		indexes := s.ParseIndexes()
		for _, name := range entry.indexes {
			found := false
			for _, index := range indexes {
				if index.Name == name {
					found = true
				}
			}
			require.True(t, found, "%s.%s", s.Table, name)
		}
	}
	s, err := schema.Parse(&MarketBarModel{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, field := range []string{"Open", "High", "Low", "Close", "Amount"} {
		require.Equal(t, "bigint", s.FieldsByName[field].TagSettings["TYPE"])
	}
	for _, field := range []string{"ValidFromVersion", "ValidToVersion"} {
		require.Equal(t, "bigint unsigned", s.FieldsByName[field].TagSettings["TYPE"])
	}
	for _, index := range s.ParseIndexes() {
		if index.Name == "idx_bar_current" {
			require.Equal(t, []string{"timeframe", "close_time", "instrument_id"}, []string{index.Fields[0].DBName, index.Fields[1].DBName, index.Fields[2].DBName})
		}
	}
	require.Error(t, Migrate(nil))
	actionSchema, err := schema.Parse(&CorporateActionModel{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	// Source IDs are byte-exact: case and trailing spaces cannot silently collide.
	require.Equal(t, "varbinary(128)", actionSchema.FieldsByName["SourceEventID"].TagSettings["TYPE"])
}

func TestOrderAndFillSchemaCanRestoreDomainProvenance(t *testing.T) {
	order, err := schema.Parse(&BacktestOrderModel{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"CreatedTime", "AttemptedAt", "Reason"} {
		require.NotNil(t, order.FieldsByName[name], name)
	}
	require.Equal(t, "datetime(6)", order.FieldsByName["CreatedTime"].TagSettings["TYPE"])
	require.Equal(t, "datetime(6)", order.FieldsByName["AttemptedAt"].TagSettings["TYPE"])
	require.True(t, order.FieldsByName["CreatedTime"].NotNull)
	require.False(t, order.FieldsByName["AttemptedAt"].NotNull)
	require.Equal(t, "longblob", order.FieldsByName["OrderID"].TagSettings["TYPE"])
	trade, err := schema.Parse(&BacktestTradeModel{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	require.NotNil(t, trade.FieldsByName["OrderID"])
	require.Equal(t, "longblob", trade.FieldsByName["FillID"].TagSettings["TYPE"])
}
