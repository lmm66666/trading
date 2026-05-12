package config

type Config struct {
	DB DB `yaml:"DB"`
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
