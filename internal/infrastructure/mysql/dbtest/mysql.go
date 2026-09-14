//go:build integration || deployment

// Package dbtest provides isolated real-MySQL integration fixtures.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/goccy/go-yaml"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"trading/config"
)

const Target = "mysql-8.4-amd64-remote"

func Targets() []string {
	return []string{Target}
}

func newDatabaseName() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "trading_test_" + hex.EncodeToString(random), nil
}

func isolatedConfig(source *driver.Config, databaseName string) *driver.Config {
	result := *source
	result.DBName = databaseName
	result.ParseTime = true
	result.Loc = time.UTC
	return &result
}

func repositoryConfigPath() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate repository config failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "config.yaml")), nil
}

// ConfiguredMySQLConfig reads the local application configuration used by
// deployment and integration checks. Callers must not log the returned value.
func ConfiguredMySQLConfig() (*driver.Config, error) {
	path, err := repositoryConfigPath()
	if err != nil {
		return nil, err
	}
	return loadConfiguredMySQL(path)
}

func loadConfiguredMySQL(path string) (*driver.Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("read local database configuration failed")
	}
	var wrapper struct {
		Config config.Config `yaml:"Config"`
	}
	if err := yaml.Unmarshal(body, &wrapper); err != nil {
		return nil, errors.New("parse local database configuration failed")
	}
	database := wrapper.Config.DB
	if database.Host == "" || database.Port < 1 || database.Port > 65535 || database.User == "" || database.DBName == "" {
		return nil, errors.New("local database configuration is incomplete")
	}
	result := driver.NewConfig()
	result.User = database.User
	result.Passwd = database.Password
	result.Net = "tcp"
	result.Addr = net.JoinHostPort(database.Host, strconv.Itoa(database.Port))
	result.DBName = database.DBName
	result.ParseTime = true
	result.Loc = time.UTC
	result.Timeout = 10 * time.Second
	result.ReadTimeout = 30 * time.Second
	result.WriteTimeout = 30 * time.Second
	return result, nil
}

func createIsolatedDatabase(t *testing.T, ctx context.Context, adminDB *sql.DB, databaseName string, testSQLDB **sql.DB) error {
	t.Helper()
	t.Cleanup(func() {
		if *testSQLDB != nil {
			if err := (*testSQLDB).Close(); err != nil {
				t.Errorf("close isolated database %s failed", databaseName)
			}
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		dropStatement := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName)
		if _, err := adminDB.ExecContext(cleanupCtx, dropStatement); err != nil {
			t.Errorf("drop isolated database %s failed", databaseName)
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("close remote MySQL administration connection failed")
		}
	})

	createStatement := fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)
	_, err := adminDB.ExecContext(ctx, createStatement)
	return err
}

// OpenIsolatedMySQL validates the approved remote target, creates a fresh
// database for the current test, and removes it during cleanup.
func OpenIsolatedMySQL(t *testing.T, target string) *gorm.DB {
	t.Helper()
	if target != Target {
		t.Fatalf("unsupported MySQL integration target %q", target)
	}
	configured, err := ConfiguredMySQLConfig()
	if err != nil {
		t.Fatal("load local MySQL test configuration failed")
	}
	adminConfig := *configured
	adminConfig.DBName = ""
	adminDB, err := sql.Open("mysql", adminConfig.FormatDSN())
	if err != nil {
		t.Fatal("open remote MySQL administration connection failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatal("ping remote MySQL failed")
	}
	var version, architecture string
	if err := adminDB.QueryRowContext(ctx, "SELECT VERSION(), @@version_compile_machine").Scan(&version, &architecture); err != nil {
		_ = adminDB.Close()
		t.Fatal("query remote MySQL compatibility failed")
	}
	if !strings.HasPrefix(version, "8.4.") {
		_ = adminDB.Close()
		t.Fatalf("remote MySQL version %q is not supported; require 8.4.x", version)
	}
	if architecture != "x86_64" {
		_ = adminDB.Close()
		t.Fatalf("remote MySQL architecture %q is not supported; require x86_64", architecture)
	}

	databaseName, err := newDatabaseName()
	if err != nil {
		_ = adminDB.Close()
		t.Fatal("generate isolated database name failed")
	}
	var testSQLDB *sql.DB
	if err := createIsolatedDatabase(t, ctx, adminDB, databaseName, &testSQLDB); err != nil {
		t.Fatalf("create isolated database %s failed", databaseName)
	}

	testConfig := isolatedConfig(configured, databaseName)
	db, err := gorm.Open(gormmysql.Open(testConfig.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated database %s failed", databaseName)
	}
	testSQLDB, err = db.DB()
	if err != nil {
		t.Fatalf("access isolated database %s connection failed", databaseName)
	}
	if err := testSQLDB.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated database %s failed", databaseName)
	}
	return db
}
