//go:build integration

package dbtest

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
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

func TestLoadConfiguredMySQLReadsApplicationConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := []byte("Config:\n  DB:\n    Host: example.invalid\n    Port: 3307\n    User: test-user\n    Password: test-password\n    DBName: production\n")
	require.NoError(t, os.WriteFile(path, body, 0o600))

	config, err := loadConfiguredMySQL(path)
	require.NoError(t, err)
	require.Equal(t, "tcp", config.Net)
	require.Equal(t, "example.invalid:3307", config.Addr)
	require.Equal(t, "test-user", config.User)
	require.Equal(t, "test-password", config.Passwd)
	require.Equal(t, "production", config.DBName)
	require.True(t, config.ParseTime)
	require.Equal(t, time.UTC, config.Loc)
}

func TestIsolatedConfigReplacesDatabaseWithoutMutatingSource(t *testing.T) {
	source := driver.NewConfig()
	source.DBName = "production"
	source.ParseTime = false

	config := isolatedConfig(source, "trading_test_0123456789abcdef0123456789abcdef")
	require.Equal(t, "trading_test_0123456789abcdef0123456789abcdef", config.DBName)
	require.True(t, config.ParseTime)
	require.Equal(t, time.UTC, config.Loc)
	require.Equal(t, "production", source.DBName)
}

func TestCreateIsolatedDatabaseCleansUpAfterUncertainCreateError(t *testing.T) {
	adminDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	databaseName := "trading_test_0123456789abcdef0123456789abcdef"
	mock.ExpectExec("^CREATE DATABASE `" + databaseName + "`").WillReturnError(errors.New("response lost"))
	mock.ExpectExec("^DROP DATABASE IF EXISTS `" + databaseName + "`$").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectClose()

	var testSQLDB *sql.DB
	t.Run("create response is uncertain", func(t *testing.T) {
		err := createIsolatedDatabase(t, context.Background(), adminDB, databaseName, &testSQLDB)
		require.Error(t, err)
	})
	require.NoError(t, mock.ExpectationsWereMet())
}
