package main

import (
	"errors"
	"net"
	"strconv"
	"trading/api"
	"trading/config"
	"trading/internal/application"
)

type serviceSettings struct {
	role    string
	address string
	client  *api.UpdaterClient
}

// resolveServiceConfig validates before opening a database or starting work.
func resolveServiceConfig(role string, cfg *config.Config) (serviceSettings, error) {
	settings := serviceSettings{role: role, address: cfg.Server.ListenAddress}
	if role != "updater" && role != "workbench" {
		return settings, errors.New("service must be updater or workbench")
	}
	if cfg.DB.Host == "" || cfg.DB.Port < 1 || cfg.DB.Port > 65535 || cfg.DB.User == "" || cfg.DB.DBName == "" {
		return settings, errors.New("database configuration is incomplete")
	}
	if err := api.ValidateUpdaterToken(cfg.Updater.Token); err != nil {
		return settings, err
	}
	if settings.address == "" {
		settings.address = "127.0.0.1:8080"
		if role == "updater" {
			settings.address = ":8081"
		}
	}
	_, port, err := net.SplitHostPort(settings.address)
	if err != nil {
		return settings, errors.New("server listen address is invalid")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return settings, errors.New("server listen port is invalid")
	}
	if role == "updater" {
		if _, err := resolveMarketConfig(cfg.Market); err != nil {
			return settings, err
		}
		if cfg.Worker.ScanBatchSize > application.MaxMarketWorkers {
			return settings, errors.New("market worker count is out of bounds")
		}
	} else {
		if _, err := resolveWorkerConfig(cfg.Worker); err != nil {
			return settings, err
		}
		settings.client, err = api.NewUpdaterClient(cfg.Updater.URL, cfg.Updater.Token)
		if err != nil {
			return settings, err
		}
	}
	return settings, nil
}
