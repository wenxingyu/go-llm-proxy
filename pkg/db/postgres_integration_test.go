package db

import (
	"context"
	"testing"
	"time"

	"go-llm-server/internal/config"
)

// To run: go test ./pkg/db -run TestInsertRequestCache_Integration -v
func TestInsertRequestCache_Integration(t *testing.T) {

	cfg := config.DatabaseConfig{
		Host:            "192.168.70.128",
		Port:            5432,
		User:            "postgres",
		Password:        "postgres_password",
		DBName:          "postgres",
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30,
	}

	pg, err := NewPostgres(cfg)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pg.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	entry := &RequestCache{
		Key:       "test",
		Request:   "{\"prompt\":\"hello\"}",
		ModelName: "gpt-4o-mini",
		Response:  "{\"text\":\"hi\"}",
	}

	if err := pg.InsertRequestCache(ctx, entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if entry.ID == 0 {
		t.Fatalf("expected returned id to be set, got 0")
	}
}
