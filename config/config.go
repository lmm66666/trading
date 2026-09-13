package config

type Config struct {
	DB     DB           `yaml:"DB"`
	Worker WorkerConfig `yaml:"Worker"`
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
