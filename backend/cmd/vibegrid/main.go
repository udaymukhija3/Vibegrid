package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vibegrid/vibegrid/backend/internal/vibegrid"
)

func main() {
	addr := env("VIBEGRID_ADDR", ":8081")
	timeZone := env("VIBEGRID_TIMEZONE", "Asia/Kolkata")

	store := vibegrid.NewMemoryAttemptStore()
	server := vibegrid.NewServer(vibegrid.ServerConfig{
		Puzzles:  vibegrid.SeedPuzzles(),
		Store:    store,
		Clock:    time.Now,
		TimeZone: timeZone,
	})

	log.Printf("vibegrid backend listening on %s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
