//go:build integration

// Package dbtest provides real MySQL integration fixtures. Docker failures fail
// tests explicitly; they are never reported as skipped/passing verification.
package dbtest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	container "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func StartMySQL(t *testing.T, image string) *gorm.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := container.Run(ctx, image, container.WithDatabase("kernel"), container.WithUsername("kernel"), container.WithPassword("test-only-password"))
	require.NoError(t, err, "Docker/MySQL integration environment is required")
	t.Cleanup(func() { require.NoError(t, c.Terminate(context.Background())) })
	dsn, err := c.ConnectionString(ctx, "parseTime=true", "loc=UTC")
	require.NoError(t, err)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}
