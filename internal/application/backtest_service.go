package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"trading/internal/backtest"
	"trading/internal/market"
	"trading/internal/port"
	"trading/internal/strategy"
)

// ComputeConfig binds executable code to a reproducible version. Clock is used
// only for telemetry and retry scheduling; market windows are explicit inputs.
type ComputeConfig struct {
	EngineVersion string
	Clock         func() time.Time
	Telemetry     port.Telemetry
}
type BacktestEngine interface {
	Run(context.Context, backtest.Input) (backtest.Result, error)
}
type BacktestService struct {
	registry *strategy.Registry
	engine   BacktestEngine
	market   port.MarketData
	queue    port.JobQueue
	store    port.RunStore
	config   ComputeConfig
}

func NewBacktestService(registry *strategy.Registry, engine BacktestEngine, data port.MarketData, queue port.JobQueue, store port.RunStore, config ComputeConfig) (*BacktestService, error) {
	if nilComputeDependency(registry) || nilComputeDependency(engine) || nilComputeDependency(data) || nilComputeDependency(queue) || nilComputeDependency(store) {
		return nil, invalidRequest("compute dependencies are required")
	}
	if err := config.defaults(); err != nil {
		return nil, err
	}
	return &BacktestService{registry, engine, data, queue, store, config}, nil
}
func (c *ComputeConfig) defaults() error {
	if err := port.ValidateIdentity(c.EngineVersion, "engine version", port.MaxEngineVersionBytes, false); err != nil {
		return err
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	return nil
}
func nilComputeDependency(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return r.Kind() == reflect.Ptr && r.IsNil()
}

func (s *BacktestService) Create(ctx context.Context, req BacktestRequest) (port.Run, error) {
	if err := ctx.Err(); err != nil {
		return port.Run{}, err
	}
	req.Start = req.Start.UTC()
	req.End = req.End.UTC()
	if err := req.Validate(); err != nil {
		return port.Run{}, err
	}
	definition, params, err := resolveComputeStrategy(s.registry, req.StrategyID, req.StrategyVersion, req.Parameters)
	if err != nil {
		return port.Run{}, err
	}
	req.Parameters = params
	if previous, found, err := previousSubmission(ctx, s.queue, port.RunBacktest, req.IdempotencyKey); err != nil {
		return port.Run{}, err
	} else if found {
		return reuseBacktestSubmission(s.registry, req, previous)
	}
	version, err := s.market.LatestCompleteVersion(ctx)
	if err != nil {
		return port.Run{}, err
	}
	if version == 0 {
		return port.Run{}, port.ErrMarketDataNotFound
	}
	hash, err := canonicalInputHash(req, definition, version, s.config.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	run, err := enqueueCompute(ctx, s.queue, port.RunBacktest, req.StrategyID, req.StrategyVersion, req.IdempotencyKey, hash, version, s.config.EngineVersion, req)
	if err == nil {
		return run, nil
	}
	winner, found, err := collidedSubmission(ctx, s.queue, port.RunBacktest, req.IdempotencyKey, err)
	if !found {
		return port.Run{}, err
	}
	return reuseBacktestSubmission(s.registry, req, winner)
}

func previousSubmission(ctx context.Context, queue port.JobQueue, kind port.RunKind, key string) (port.Run, bool, error) {
	reader, ok := queue.(port.IdempotentRunReader)
	if !ok {
		return port.Run{}, false, nil
	}
	run, err := reader.FindByIdempotency(ctx, kind, key)
	if errors.Is(err, port.ErrRunNotFound) {
		return port.Run{}, false, nil
	}
	return run, err == nil, err
}
func (s *BacktestService) Cancel(ctx context.Context, id string) error {
	return s.queue.RequestCancel(ctx, id)
}

func resolveComputeStrategy(registry *strategy.Registry, id, version string, params map[string]float64) (strategy.Definition, map[string]float64, error) {
	instance, err := registry.Resolve(id, version, params)
	if err != nil {
		return strategy.Definition{}, nil, err
	}
	definition := instance.Definition()
	if definition.WarmupBars > port.MaxLookbackBars {
		return strategy.Definition{}, nil, invalidRequest("strategy warmup exceeds limit")
	}
	resolved := make(map[string]float64, len(definition.Parameters))
	for name, spec := range definition.Parameters {
		resolved[name] = spec.Default
	}
	for name, value := range params {
		if value == 0 {
			value = 0
		}
		resolved[name] = value
	}
	return definition, resolved, nil
}
func enqueueCompute(ctx context.Context, queue port.JobQueue, kind port.RunKind, id, version, key, hash string, dataVersion market.DataVersion, engineVersion string, request any) (port.Run, error) {
	encoded, err := canonicalJSON(request)
	if err != nil {
		return port.Run{}, err
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return port.Run{}, err
	}
	run := port.Run{ID: hex.EncodeToString(random), Kind: kind, Status: port.RunPending, StrategyID: id, StrategyVersion: version, IdempotencyKey: key, InputHash: hash, DataVersion: dataVersion, EngineVersion: engineVersion, RequestJSON: encoded}
	if err := run.Validate(); err != nil {
		return port.Run{}, err
	}
	return queue.Enqueue(ctx, run)
}
func canonicalInputHash(req BacktestRequest, definition strategy.Definition, version market.DataVersion, engineVersion string) (string, error) {
	req.IdempotencyKey = ""
	req.Start = req.Start.UTC()
	req.End = req.End.UTC()
	return computeHash(struct {
		Request       BacktestRequest
		Definition    strategy.Definition
		DataVersion   market.DataVersion
		EngineVersion string
	}{req, definition, version, engineVersion})
}
func computeHash(v any) (string, error) {
	data, err := canonicalJSON(v)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// canonicalJSON sorts every object key and uses non-exponent shortest exact
// float64 decimals. Integer identifiers/money never pass through float64.
func canonicalJSON(v any) ([]byte, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	var write func(any) error
	write = func(value any) error {
		switch x := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					out.WriteByte(',')
				}
				b, _ := json.Marshal(k)
				out.Write(b)
				out.WriteByte(':')
				if err := write(x[k]); err != nil {
					return err
				}
			}
			out.WriteByte('}')
		case []any:
			out.WriteByte('[')
			for i, v := range x {
				if i > 0 {
					out.WriteByte(',')
				}
				if err := write(v); err != nil {
					return err
				}
			}
			out.WriteByte(']')
		case json.Number:
			n := x.String()
			if strings.ContainsAny(n, ".eE") {
				f, err := strconv.ParseFloat(n, 64)
				if err != nil {
					return err
				}
				n = strconv.FormatFloat(f, 'f', -1, 64)
			}
			if n == "-0" {
				n = "0"
			}
			out.WriteString(n)
		default:
			b, err := json.Marshal(x)
			if err != nil {
				return err
			}
			out.Write(b)
		}
		return nil
	}
	if err = write(value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
