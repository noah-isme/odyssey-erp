package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dsn := getenv("PG_DSN", "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	password, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	_, err = pool.Exec(ctx, `INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
        VALUES ($1, $2, TRUE, NOW(), NOW())
        ON CONFLICT (email) DO NOTHING`, "admin@odyssey.local", string(password))
	if err != nil {
		log.Fatalf("seed user: %v", err)
	}
	fmt.Println("Seed complete at", time.Now().Format(time.RFC3339))
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
