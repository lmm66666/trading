//go:build deployment

package mysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestDeploymentMySQLCompatibility(t *testing.T) {
	dsn := os.Getenv("TRADING_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("TRADING_TEST_MYSQL_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open deployment MySQL failed")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("ping deployment MySQL failed")
	}
	var version, architecture string
	if err := db.QueryRowContext(ctx, "SELECT VERSION(), @@version_compile_machine").Scan(&version, &architecture); err != nil {
		t.Fatal("query deployment MySQL compatibility failed")
	}
	t.Logf("deployment MySQL version=%s architecture=%s", version, architecture)
	if !strings.HasPrefix(version, "8.4.") {
		t.Fatalf("deployment MySQL version=%q, want prefix 8.4", version)
	}
	if architecture != "x86_64" {
		t.Fatalf("deployment MySQL architecture=%q, want x86_64", architecture)
	}
}
