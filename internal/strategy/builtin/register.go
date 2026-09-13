// Package builtin contains the compiled strategies shipped with the service.
package builtin

import "trading/internal/strategy"

// RegisterAll installs every built-in strategy version supported by this
// binary. Registration is deliberately explicit so upgrades cannot silently
// replace an existing strategy version.
func RegisterAll(registry *strategy.Registry) error {
	for _, item := range []struct {
		id      string
		factory strategy.Factory
	}{
		{id: dailyB1ID, factory: NewDailyB1},
		{id: weeklyB1ID, factory: NewWeeklyB1},
		{id: bottomSurgeID, factory: NewBottomSurge},
	} {
		if err := registry.Register(item.id, strategyVersion, item.factory); err != nil {
			return err
		}
	}
	return nil
}
