package main

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/http/httptest"
	"testing"
	"trading/api"
	"trading/config"
)

func serviceConfig() *config.Config {
	return &config.Config{DB: config.DB{Host: "localhost", Port: 3306, User: "test", DBName: "test"}, Updater: config.UpdaterConfig{URL: "http://127.0.0.1:8081", Token: "0123456789abcdef0123456789abcdef"}}
}

func TestServiceRoleValidatedBeforeConfigurationRead(t *testing.T) {
	for _, role := range []string{"", "all", "invalid"} {
		require.ErrorContains(t, run(context.Background(), "missing-config", role), "service must be updater or workbench")
	}
}
func TestServiceConfigDefaultsAndRoleSpecificValidation(t *testing.T) {
	for _, tc := range []struct{ role, address string }{{"updater", ":8081"}, {"workbench", "127.0.0.1:8080"}} {
		cfg := serviceConfig()
		settings, err := resolveServiceConfig(tc.role, cfg)
		require.NoError(t, err)
		require.Equal(t, tc.address, settings.address)
	}
	cfg := serviceConfig()
	cfg.Market.StockRequestIntervalSeconds = 1
	_, err := resolveServiceConfig("workbench", cfg)
	require.NoError(t, err)
	_, err = resolveServiceConfig("updater", cfg)
	require.Error(t, err)
	cfg = serviceConfig()
	cfg.Worker.Count = 100
	_, err = resolveServiceConfig("updater", cfg)
	require.NoError(t, err)
	_, err = resolveServiceConfig("workbench", cfg)
	require.Error(t, err)
	cfg = serviceConfig()
	cfg.Updater.URL = ""
	_, err = resolveServiceConfig("updater", cfg)
	require.NoError(t, err)
	_, err = resolveServiceConfig("workbench", cfg)
	require.Error(t, err)
	cfg = serviceConfig()
	cfg.Updater.Token = "short"
	_, err = resolveServiceConfig("updater", cfg)
	require.Error(t, err)
	cfg = serviceConfig()
	cfg.Server.ListenAddress = "broken"
	_, err = resolveServiceConfig("workbench", cfg)
	require.Error(t, err)
	cfg = serviceConfig()
	cfg.DB.DBName = ""
	_, err = resolveServiceConfig("workbench", cfg)
	require.Error(t, err)
}
func TestServiceCompositionIsolatesRuntimeResponsibilities(t *testing.T) {
	conn, _, err := sqlmock.New()
	require.NoError(t, err)
	defer conn.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	cfg := serviceConfig()
	cfg.Market.FuturesEnabled = true
	settings, err := resolveServiceConfig("workbench", cfg)
	require.NoError(t, err)
	workbench, err := newServiceKernel(context.Background(), db, cfg, settings)
	require.NoError(t, err)
	require.NotNil(t, workbench.workers)
	require.Nil(t, workbench.marketScheduler)
	require.Nil(t, workbench.futuresScheduler)
	require.Nil(t, workbench.services.MarketIngestion)
	require.Nil(t, workbench.services.MarketTrigger)
	require.NotNil(t, workbench.services.RemoteRefresh)
	w := httptest.NewRecorder()
	api.NewRouter(workbench.services).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/strategies", nil))
	require.Equal(t, 200, w.Code)
	settings, err = resolveServiceConfig("updater", cfg)
	require.NoError(t, err)
	updater, err := newServiceKernel(context.Background(), db, cfg, settings)
	require.NoError(t, err)
	require.Nil(t, updater.workers)
	require.Nil(t, updater.services.Backtests)
	require.Nil(t, updater.services.Scans)
	require.Nil(t, updater.services.RemoteRefresh)
	require.NotNil(t, updater.marketScheduler)
	require.NotNil(t, updater.futuresScheduler)
}
