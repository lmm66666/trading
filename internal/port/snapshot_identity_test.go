package port_test

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"trading/internal/port"
)

func TestSnapshotKeyPreservesAndValidatesExactSnapshotID(t *testing.T) {
	key := port.SnapshotKey{StrategyID: "strategy", StrategyVersion: "v1", ParametersHash: "hash"}
	for _, id := range []string{"", "Snapshot", "snapshot", "snapshot ", strings.Repeat("x", 64)} {
		key.SnapshotID = id
		require.NoError(t, key.Validate())
		raw, err := json.Marshal(key)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(raw, &wire))
		if id == "" {
			require.NotContains(t, wire, "snapshot_id")
		} else {
			require.Equal(t, id, wire["snapshot_id"])
		}
		var restored port.SnapshotKey
		require.NoError(t, json.Unmarshal(raw, &restored))
		require.Equal(t, id, restored.SnapshotID)
	}
	for _, id := range []string{" ", strings.Repeat("x", 65), string([]byte{255})} {
		key.SnapshotID = id
		require.ErrorIs(t, key.Validate(), port.ErrInvalidPortValue)
	}
}
