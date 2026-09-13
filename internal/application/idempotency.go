package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"trading/internal/port"
	"trading/internal/strategy"
)

// 只在确定的唯一键碰撞后重读胜出任务，其他存储错误原样传播。
func collidedSubmission(ctx context.Context, queue port.JobQueue, kind port.RunKind, key string, enqueueErr error) (port.Run, bool, error) {
	if !errors.Is(enqueueErr, port.ErrIdempotencyConflict) {
		return port.Run{}, false, enqueueErr
	}
	run, found, err := previousSubmission(ctx, queue, kind, key)
	if err != nil {
		return port.Run{}, false, err
	}
	if !found {
		return port.Run{}, false, enqueueErr
	}
	return run, true, nil
}

// 严格验证完整保存字段。JSON数据库可以改变键顺序和空格，但不能
// 静默忽略未知字段、大小写变体或把缺失字段当作合法的零值。
func decodePinnedRequest(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalidRequest("invalid saved request")
	}
	actual, err := canonicalJSON(json.RawMessage(data))
	if err != nil {
		return invalidRequest("invalid saved request")
	}
	expected, err := canonicalJSON(target)
	if err != nil || !bytes.Equal(actual, expected) {
		return invalidRequest("incomplete saved request")
	}
	return nil
}
func validateWinner(run port.Run, kind port.RunKind, key string) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Kind != kind || run.IdempotencyKey != key {
		return invalidRequest("idempotency lookup returned unrelated run")
	}
	return nil
}
func sameRequestIntent(left, right any) error {
	a, err := canonicalJSON(left)
	if err != nil {
		return err
	}
	b, err := canonicalJSON(right)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, b) {
		return port.ErrIdempotencyConflict
	}
	return nil
}

func reuseBacktestSubmission(registry *strategy.Registry, request BacktestRequest, winner port.Run) (port.Run, error) {
	if err := validateWinner(winner, port.RunBacktest, request.IdempotencyKey); err != nil {
		return port.Run{}, err
	}
	var saved BacktestRequest
	if err := decodePinnedRequest(winner.RequestJSON, &saved); err != nil {
		return port.Run{}, err
	}
	if err := saved.Validate(); err != nil {
		return port.Run{}, err
	}
	definition, _, err := resolveComputeStrategy(registry, winner.StrategyID, winner.StrategyVersion, saved.Parameters)
	if err != nil {
		return port.Run{}, err
	}
	hash, err := canonicalInputHash(saved, definition, winner.DataVersion, winner.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	if hash != winner.InputHash || saved.StrategyID != winner.StrategyID || saved.StrategyVersion != winner.StrategyVersion {
		return port.Run{}, invalidRequest("saved backtest inputs changed")
	}
	if err := sameRequestIntent(request, saved); err != nil {
		return port.Run{}, err
	}
	return winner, nil
}
func reuseScanSubmission(registry *strategy.Registry, request ScanRequest, winner port.Run) (port.Run, error) {
	if err := validateWinner(winner, port.RunScan, request.IdempotencyKey); err != nil {
		return port.Run{}, err
	}
	var saved scanInput
	if err := decodePinnedRequest(winner.RequestJSON, &saved); err != nil {
		return port.Run{}, err
	}
	if err := saved.Validate(); err != nil {
		return port.Run{}, err
	}
	definition, _, err := resolveComputeStrategy(registry, winner.StrategyID, winner.StrategyVersion, saved.Parameters)
	if err != nil {
		return port.Run{}, err
	}
	hash, err := scanInputHash(saved, definition, winner.DataVersion, winner.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	parametersHash, err := scanParametersHash(saved, definition, winner.EngineVersion)
	if err != nil {
		return port.Run{}, err
	}
	if hash != winner.InputHash || parametersHash != saved.ParametersHash || saved.StrategyID != winner.StrategyID || saved.StrategyVersion != winner.StrategyVersion {
		return port.Run{}, invalidRequest("saved scan inputs changed")
	}
	if err := sameRequestIntent(request, saved.ScanRequest); err != nil {
		return port.Run{}, err
	}
	return winner, nil
}
