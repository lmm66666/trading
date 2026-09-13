package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"trading/config"
	kernel "trading/internal/infrastructure/mysql"
	"trading/internal/port"
)

func TestCommandDefaultsToDryRun(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), nil, &output, func(_ context.Context, path string, opts kernel.MigrationOptions) (kernel.MigrationReport, error) {
		require.Equal(t, "config.yaml", path)
		require.True(t, opts.DryRun)
		require.Equal(t, 1000, opts.BatchSize)
		return kernel.MigrationReport{Quality: port.DataComplete, Digest: strings.Repeat("a", 64)}, nil
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), `"Quality":"COMPLETE"`)
}
func TestCommandApplyAndConfigAreExplicit(t *testing.T) {
	err := run(context.Background(), []string{"-config", "fixture.yaml", "-dry-run=false", "-batch-size", "12"}, &bytes.Buffer{}, func(_ context.Context, path string, opts kernel.MigrationOptions) (kernel.MigrationReport, error) {
		require.Equal(t, "fixture.yaml", path)
		require.False(t, opts.DryRun)
		require.Equal(t, 12, opts.BatchSize)
		return kernel.MigrationReport{}, nil
	})
	require.NoError(t, err)
}
func TestCommandNeverPrintsDependencyErrorsOrSensitiveArguments(t *testing.T) {
	for _, args := range [][]string{{"-batch-size", "0"}, {"-unknown=password@secret"}, {"-batch-size", "bad"}, {"unexpected"}, {}} {
		var output bytes.Buffer
		err := run(context.Background(), args, &output, func(context.Context, string, kernel.MigrationOptions) (kernel.MigrationReport, error) {
			return kernel.MigrationReport{}, errors.New("root:password@tcp(secret)/db")
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
		require.NotContains(t, output.String(), "password")
		require.NotContains(t, output.String(), "secret")
	}
}
func TestCommandReportsIncompleteAndReturnsFailure(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), nil, &output, func(context.Context, string, kernel.MigrationOptions) (kernel.MigrationReport, error) {
		return kernel.MigrationReport{Version: 1, Quality: port.DataIncomplete}, kernel.ErrMigrationIncomplete
	})
	require.ErrorIs(t, err, kernel.ErrMigrationIncomplete)
	require.Contains(t, output.String(), `"BacktestEnabled":false`)
}
func TestCommandMissingConfigDoesNotEchoPath(t *testing.T) {
	_, err := execute(context.Background(), "/nonexistent/password", kernel.MigrationOptions{DryRun: true, BatchSize: 1})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "password")
}

func TestCommandConfigurationAndConnectionFailuresAreRedacted(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	for _, body := range []string{"DB: [password", "DB: {User: secret}", fmt.Sprintf("DB:\n  Host: 127.0.0.1\n  Port: %d\n  User: secret\n  Password: password\n  DBName: test\n", port)} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
		_, err := execute(context.Background(), path, kernel.MigrationOptions{DryRun: true, BatchSize: 1})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
		require.NotContains(t, err.Error(), "secret")
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
func TestCommandPropagatesOutputFailureWithoutRawError(t *testing.T) {
	err := run(context.Background(), nil, failedOutput{}, func(context.Context, string, kernel.MigrationOptions) (kernel.MigrationReport, error) {
		return kernel.MigrationReport{}, nil
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "output unavailable")
}

func TestDryRunCompositionNeverMigratesSchemaAndClosesConnection(t *testing.T) {
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(driver.New(driver.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("DB: {Host: db, Port: 3306, User: account, Password: private, DBName: trading}"), 0600))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*t_stock_info").WillReturnError(errors.New("private SQL details"))
	mock.ExpectRollback()
	mock.ExpectClose()
	_, err = executeWithDatabase(context.Background(), path, kernel.MigrationOptions{DryRun: true, BatchSize: 100}, func(cfg config.DB) (*gorm.DB, error) { require.Equal(t, "private", cfg.Password); return db, nil })
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Error(t, conn.Ping())
}
