package main

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed web
var webFiles embed.FS

func main() {
	dbPath := env("KOMPAGNON_DB", "kompagnon.db")
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Fatal(err)
		}
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		log.Fatal(err)
	}
	if err := migrate(db); err != nil {
		log.Fatal(err)
	}

	port, err := strconv.Atoi(env("PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		log.Fatal("PORT must be a valid TCP port")
	}

	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	app := newServer(db, env("KOMPAGNON_COOKIE_SECURE", "false") == "true", static)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           app.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Kompagnon listening on http://localhost:%d", port)
	log.Fatal(server.ListenAndServe())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
