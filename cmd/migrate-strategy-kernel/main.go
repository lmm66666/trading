// migrate-strategy-kernel 在维护窗口内验证并迁移旧行情；默认仅执行只读校验。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	sqldriver "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"trading/config"
	kernel "trading/internal/infrastructure/mysql"
	"trading/pkg/broker"
)

type migrationRunner func(context.Context, string, kernel.MigrationOptions) (kernel.MigrationReport, error)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, execute); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer, runner migrationRunner) error {
	flags := flag.NewFlagSet("migrate-strategy-kernel", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // Flag errors may otherwise repeat arbitrary input.
	dryRun := flags.Bool("dry-run", true, "只读验证，不写入数据库")
	batchSize := flags.Int("batch-size", 1000, "每批旧表主键记录数（1–10000）")
	configPath := flags.String("config", "config.yaml", "运行配置文件")
	if err := flags.Parse(args); err != nil {
		return errors.New("迁移参数无效；支持 -config、-dry-run、-batch-size")
	}
	if flags.NArg() != 0 || *batchSize < 1 || *batchSize > 10000 || *configPath == "" {
		return errors.New("迁移参数无效；批次大小须为 1–10000")
	}
	report, err := runner(ctx, *configPath, kernel.MigrationOptions{DryRun: *dryRun, BatchSize: *batchSize})
	if encodeErr := json.NewEncoder(out).Encode(report); encodeErr != nil {
		return errors.New("无法输出迁移报告")
	}
	if err != nil && !errors.Is(err, kernel.ErrMigrationIncomplete) {
		return errors.New("迁移失败；请检查配置、数据库连接、维护窗口和迁移检查点")
	}
	if err != nil {
		return kernel.ErrMigrationIncomplete
	}
	return nil
}

// Connection setup is separate from data.New: dry-run must not AutoMigrate old
// or new tables. Both the GORM logger and returned errors redact driver details.
func execute(ctx context.Context, path string, opts kernel.MigrationOptions) (kernel.MigrationReport, error) {
	return executeWithDatabase(ctx, path, opts, openDatabase)
}

func executeWithDatabase(ctx context.Context, path string, opts kernel.MigrationOptions, connect func(config.DB) (*gorm.DB, error)) (kernel.MigrationReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return kernel.MigrationReport{}, errors.New("无法读取迁移配置")
	}
	defer file.Close()
	var wrapper struct {
		Config config.Config `yaml:"Config"`
	}
	decoder := yaml.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.KnownFields(true)
	if err := decoder.Decode(&wrapper); err != nil {
		return kernel.MigrationReport{}, errors.New("迁移配置格式无效")
	}
	cfg := wrapper.Config
	if cfg.DB.Host == "" || cfg.DB.Port < 1 || cfg.DB.Port > 65535 || cfg.DB.User == "" || cfg.DB.DBName == "" {
		return kernel.MigrationReport{}, errors.New("迁移数据库配置不完整")
	}
	db, err := connect(cfg.DB)
	if err != nil {
		return kernel.MigrationReport{}, errors.New("无法连接迁移数据库")
	}
	conn, err := db.DB()
	if err != nil {
		return kernel.MigrationReport{}, errors.New("无法初始化迁移数据库连接")
	}
	defer conn.Close()
	conn.SetMaxOpenConns(2)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxLifetime(5 * time.Minute)
	if !opts.DryRun {
		if err := kernel.Migrate(db.WithContext(ctx)); err != nil {
			return kernel.MigrationReport{}, errors.New("无法初始化内核表")
		}
	}
	return kernel.NewLegacyMigrator(db, broker.NewEastmoneyMarketSource()).Run(ctx, opts)
}

func openDatabase(cfg config.DB) (*gorm.DB, error) {
	dsn := sqldriver.NewConfig()
	dsn.User = cfg.User
	dsn.Passwd = cfg.Password
	dsn.Net = "tcp"
	dsn.Addr = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dsn.DBName = cfg.DBName
	dsn.ParseTime = true
	dsn.Loc = time.UTC
	dsn.Timeout = 10 * time.Second
	dsn.ReadTimeout = 30 * time.Second
	dsn.WriteTimeout = 30 * time.Second
	return gorm.Open(driver.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }})
}
