package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	"taskflow/internal/config"
	"taskflow/internal/database"
	"taskflow/internal/server"
)

func main() {
	cfg := config.Load()

	var db *gorm.DB
	var err error

	switch strings.ToLower(cfg.DBDriver) {
	case "sqlite":
		db, err = database.Open(cfg.DBPath)

	case "postgres":
		if cfg.DBPSG == "" {
			log.Fatal("DB_PSG es obligatorio para Postgres")
		}
		db, err = database.OpenPostgres(cfg.DBPSG)

	default:
		log.Fatalf("Error al conectarse a una base de datos: %s", cfg.DBDriver)
	}

	if err != nil {
		log.Fatalf("ERROR %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatal(err)
	}

	if err := database.SeedAdmin(db, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.NewRouter(db, cfg, "web"),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("TaskFlow escuchando en http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor: %v", err)
		}
	}()

	// apagado limpio con Ctrl+C
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("apagando servidor...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}
}
