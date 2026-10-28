package config

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
)

const minJWTSecretLen = 32

type Config struct {
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	HTTP     HTTP
	MySQL    MySQL
	Redis    Redis
	JWT      JWT
	Cache    Cache
	Limit    RateLimit
}

type HTTP struct {
	Addr            string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	RequestTimeout  time.Duration `env:"HTTP_REQUEST_TIMEOUT" envDefault:"10s"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

type MySQL struct {
	Host            string        `env:"MYSQL_HOST,required"`
	Port            int           `env:"MYSQL_PORT" envDefault:"3306"`
	User            string        `env:"MYSQL_USER,required"`
	Password        string        `env:"MYSQL_PASSWORD,required"`
	Database        string        `env:"MYSQL_DATABASE,required"`
	MaxOpenConns    int           `env:"MYSQL_MAX_OPEN_CONNS" envDefault:"20"`
	MaxIdleConns    int           `env:"MYSQL_MAX_IDLE_CONNS" envDefault:"10"`
	ConnMaxLifetime time.Duration `env:"MYSQL_CONN_MAX_LIFETIME" envDefault:"30m"`
}

type Redis struct {
	Addr            string        `env:"REDIS_ADDR,required"`
	Password        string        `env:"REDIS_PASSWORD"`
	DB              int           `env:"REDIS_DB" envDefault:"0"`
	Timeout         time.Duration `env:"REDIS_TIMEOUT" envDefault:"300ms"`
	BreakerFailures uint32        `env:"REDIS_BREAKER_FAILURES" envDefault:"5"`
	BreakerCooldown time.Duration `env:"REDIS_BREAKER_COOLDOWN" envDefault:"15s"`
}

type JWT struct {
	Secret string        `env:"JWT_SECRET,required"`
	TTL    time.Duration `env:"JWT_TTL" envDefault:"24h"`
}

type Cache struct {
	TasksTTL time.Duration `env:"CACHE_TASKS_TTL" envDefault:"5m"`
}

type RateLimit struct {
	Requests int           `env:"RATE_LIMIT_REQUESTS" envDefault:"100"`
	Window   time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	var errs []error
	if len(c.JWT.Secret) < minJWTSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes long", minJWTSecretLen))
	}
	if c.JWT.TTL <= 0 {
		errs = append(errs, errors.New("JWT_TTL must be positive"))
	}
	if c.Cache.TasksTTL <= 0 {
		errs = append(errs, errors.New("CACHE_TASKS_TTL must be positive"))
	}
	if c.Limit.Requests <= 0 || c.Limit.Window <= 0 {
		errs = append(errs, errors.New("RATE_LIMIT_REQUESTS and RATE_LIMIT_WINDOW must be positive"))
	}
	if c.HTTP.RequestTimeout <= 0 || c.HTTP.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("HTTP_REQUEST_TIMEOUT and HTTP_SHUTDOWN_TIMEOUT must be positive"))
	}
	if c.Redis.BreakerFailures == 0 {
		errs = append(errs, errors.New("REDIS_BREAKER_FAILURES must be positive"))
	}
	return errors.Join(errs...)
}

func LoadMySQL() (MySQL, error) {
	cfg, err := env.ParseAs[MySQL]()
	if err != nil {
		return MySQL{}, fmt.Errorf("parse env: %w", err)
	}
	return cfg, nil
}
