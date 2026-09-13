package strategy

import (
	"github.com/stretchr/testify/require"
	"testing"
	"trading/internal/market"
)

func TestDefinitionsAreSortedIndependentCopies(t *testing.T) {
	r := &Registry{}
	require.Empty(t, r.Definitions())
	for _, id := range []string{"z", "a"} {
		d := Definition{ID: id, Version: "1", PrimaryTimeframe: market.Day, Parameters: map[string]ParameterSpec{"window": {Default: 2, Min: 1, Max: 3}}}
		require.NoError(t, r.Register(id, "1", strategyFactory(d)))
	}
	list := r.Definitions()
	require.Equal(t, "a", list[0].ID)
	require.Equal(t, "z", list[1].ID)
	list[0].Parameters["window"] = ParameterSpec{}
	require.Equal(t, 2.0, r.Definitions()[0].Parameters["window"].Default)
}
