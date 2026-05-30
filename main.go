package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	log.SetOutput(os.Stderr)

	pool, err := connectDB()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	s := server.NewMCPServer("postgres-mcp", "1.0.0",
		server.WithToolCapabilities(true),
	)

	registerTools(s, pool)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func connectDB() (*pgxpool.Pool, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		host := envOrDefault("POSTGRES_HOST", "localhost")
		port := envOrDefault("POSTGRES_PORT", "5432")
		user := envOrDefault("POSTGRES_USER", "postgres")
		password := os.Getenv("POSTGRES_PASSWORD")
		dbname := envOrDefault("POSTGRES_DB", "postgres")
		sslmode := envOrDefault("POSTGRES_SSLMODE", "disable")
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, password, dbname, sslmode)
	}
	return pgxpool.New(context.Background(), dsn)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
