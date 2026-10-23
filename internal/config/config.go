package config

import (
	"log"
	"os"
	"time"
)

type Config struct {
	Port          string
	DBPath        string
	DBDriver      string
	DBPSG         string
	JWTSecret     string
	TokenTTL      time.Duration
	AdminEmail    string
	AdminPassword string
	// En desarrollo no hay servidor de correo: el token de recuperación se
	// devuelve en la respuesta para poder probar el flujo.
	DevMode bool
}

func Load() Config {
	cfg := Config{
		Port:          getenv("PORT", "8080"),
		DBPath:        getenv("DB_PATH", "taskflow.db"),
		DBDriver:      getenv("DB_DRIVER", "sqlite"),
		DBPSG:         getenv("DB_PSG", ""),
		JWTSecret:     getenv("JWT_SECRET", ""),
		TokenTTL:      12 * time.Hour,
		AdminEmail:    getenv("ADMIN_EMAIL", "admin@taskflow.local"),
		AdminPassword: getenv("ADMIN_PASSWORD", "Admin123!"),
		DevMode:       getenv("APP_ENV", "development") == "development",
	}
	if cfg.JWTSecret == "" {
		if !cfg.DevMode {
			log.Fatal("JWT_SECRET es obligatorio fuera de desarrollo")
		}
		cfg.JWTSecret = "dev-secret-cambiar-en-produccion"
		log.Println("[WARN] usando JWT_SECRET de desarrollo")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
