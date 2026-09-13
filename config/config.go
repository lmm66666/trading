package config

type Config struct {
	DB     DB           `yaml:"DB"`
	Worker WorkerConfig `yaml:"Worker"`
	Market MarketConfig `yaml:"Market"`
}

// MarketConfig controls external market-data pacing. Zero values use safe
// production defaults resolved by the composition root.
type MarketConfig struct {
	StockRequestIntervalSeconds int  `yaml:"StockRequestIntervalSeconds"`
	FuturesEnabled              bool `yaml:"FuturesEnabled"`
	FuturesRefreshIntervalHours int  `yaml:"FuturesRefreshIntervalHours"`
}

// WorkerConfig controls durable compute workers and bounded market/scan fan-out.
// Zero values are replaced with production defaults by the composition root.
type WorkerConfig struct {
	Count               int `yaml:"Count"`
	LeaseSeconds        int `yaml:"LeaseSeconds"`
	PollIntervalMillis  int `yaml:"PollIntervalMillis"`
	SyncWaitTimeoutSecs int `yaml:"SyncWaitTimeoutSecs"`
	ScanBatchSize       int `yaml:"ScanBatchSize"`
}

type DB struct {
	Host                   string `yaml:"Host"`
	Port                   int    `yaml:"Port"`
	User                   string `yaml:"User"`
	Password               string `yaml:"Password"`
	DBName                 string `yaml:"DBName"`
	MaxOpenConns           int    `yaml:"MaxOpenConns"`
	MaxIdleConns           int    `yaml:"MaxIdleConns"`
	ConnMaxLifetimeMinutes int    `yaml:"ConnMaxLifetimeMinutes"`
}
