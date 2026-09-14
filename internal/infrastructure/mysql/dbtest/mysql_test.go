//go:build integration

package dbtest

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTargetsUseOnlyRemoteMySQL84(t *testing.T) {
	require.Equal(t, []string{"mysql-8.4-amd64-remote"}, Targets())
}

func TestDatabaseNameIsControlledAndUnique(t *testing.T) {
	first, err := newDatabaseName()
	require.NoError(t, err)
	second, err := newDatabaseName()
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^trading_test_[0-9a-f]{32}$`), first)
	require.Regexp(t, regexp.MustCompile(`^trading_test_[0-9a-f]{32}$`), second)
	require.NotEqual(t, first, second)
}

func TestIsolatedConfigReplacesDatabaseAndForcesUTCTime(t *testing.T) {
	config, err := isolatedConfig("root:secret@tcp(example.invalid:3306)/production?charset=utf8mb4", "trading_test_0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	require.Equal(t, "trading_test_0123456789abcdef0123456789abcdef", config.DBName)
	require.True(t, config.ParseTime)
	require.Equal(t, time.UTC, config.Loc)
}
