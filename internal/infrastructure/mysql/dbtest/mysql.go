//go:build integration

// Package dbtest provides isolated real-MySQL integration fixtures.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
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

func isolatedConfig(rawDSN, databaseName string) (*driver.Config, error) {
	config, err := driver.ParseDSN(rawDSN)
	if err != nil {
		return nil, err
	}
	config.DBName = databaseName
	config.ParseTime = true
	config.Loc = time.UTC
	return config, nil
}

// OpenIsolatedMySQL validates the approved remote target, creates a fresh
// database for the current test, and removes it during cleanup.
func OpenIsolatedMySQL(t *testing.T, target string) *gorm.DB {
	t.Helper()
	if target != Target {
		t.Fatalf("unsupported MySQL integration target %q", target)
	}
	rawDSN := os.Getenv("TRADING_TEST_MYSQL_DSN")
	if rawDSN == "" {
		t.Fatal("TRADING_TEST_MYSQL_DSN is required")
	}

	adminConfig, err := driver.ParseDSN(rawDSN)
	if err != nil {
		t.Fatal("parse remote MySQL test DSN failed")
	}
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
	createStatement := fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)
	if _, err := adminDB.ExecContext(ctx, createStatement); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create isolated database %s failed", databaseName)
	}

	var testSQLDB *sql.DB
	t.Cleanup(func() {
		if testSQLDB != nil {
			if err := testSQLDB.Close(); err != nil {
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

	testConfig, err := isolatedConfig(rawDSN, databaseName)
	if err != nil {
		t.Fatal("build isolated MySQL test DSN failed")
	}
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
