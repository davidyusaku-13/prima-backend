package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL is required")
	}

	sourceURL := os.Getenv("MIGRATIONS_SOURCE")
	if sourceURL == "" {
		sourceURL = "file://migrations"
	}

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	m, err := migrate.New(sourceURL, dsn)
	if err != nil {
		panic(err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			fmt.Fprintf(os.Stderr, "migration source close error: %v\n", srcErr)
		}
		if dbErr != nil {
			fmt.Fprintf(os.Stderr, "migration db close error: %v\n", dbErr)
		}
	}()

	switch command {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "version":
		version, dirty, vErr := m.Version()
		if vErr != nil {
			if errors.Is(vErr, migrate.ErrNilVersion) {
				fmt.Println("version: none")
				return
			}
			panic(vErr)
		}
		fmt.Printf("version: %d dirty: %t\n", version, dirty)
		return
	default:
		panic("usage: go run ./cmd/migrate [up|down|version]")
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		panic(err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no migration changes")
		return
	}

	fmt.Printf("migration command %q completed\n", command)
}
