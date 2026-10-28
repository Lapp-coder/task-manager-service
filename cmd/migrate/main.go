package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pressly/goose/v3"

	"github.com/lapp-coder/task-manager-service/internal/config"
	"github.com/lapp-coder/task-manager-service/internal/storage/mysql"
	"github.com/lapp-coder/task-manager-service/migrations"
)

const usage = "usage: migrate up|down|status|version"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	if len(args) != 1 {
		return errors.New(usage)
	}
	cfg, err := config.LoadMySQL()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := mysql.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()

	return migrate(ctx, db, args[0])
}

func migrate(ctx context.Context, db *sql.DB, command string) error {
	provider, err := goose.NewProvider(goose.DialectMySQL, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	switch command {
	case "up":
		results, err := provider.Up(ctx)
		for _, r := range results {
			fmt.Println(r)
		}
		return wrap(command, err)
	case "down":
		result, err := provider.Down(ctx)
		if result != nil {
			fmt.Println(result)
		}
		return wrap(command, err)
	case "status":
		statuses, err := provider.Status(ctx)
		for _, s := range statuses {
			fmt.Printf("%-8s %s\n", s.State, s.Source.Path)
		}
		return wrap(command, err)
	case "version":
		version, err := provider.GetDBVersion(ctx)
		fmt.Println(version)
		return wrap(command, err)
	default:
		return errors.New(usage)
	}
}

func wrap(command string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}
	return nil
}
