package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"taskflow/internal/models"
)

// Conexion a SQLite para desarrollo y pruebas
func Open(path string) (*gorm.DB, error) {
	// foreign_keys viene apagado por defecto en SQLite
	dsn := path
	if strings.Contains(dsn, "?") {
		dsn += "&_pragma=foreign_keys(1)"
	} else {
		dsn += "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("error abriendo base de datos: %w", err)
	}
	return db, nil
}

// Conexion a Postgres para produccion
func OpenPostgres(psg string) (*gorm.DB, error) {
	db, err := gorm.Open(
		postgres.Open(psg), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})

	if err != nil {
		return nil, fmt.Errorf("error abriendo base de datos: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("error obteniendo la base de datos: %w", err)
	}

	//config del pool de las conexiones
	sqlDB.SetMaxIdleConns(6)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxLifetime(40 * time.Minute)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("error al intentar alcanzar la base de datos: %w", err)
	}

	return db, nil
}

func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&models.User{},
		&models.PasswordReset{},
		&models.Project{},
		&models.Board{},
		&models.Column{},
		&models.Task{},
		&models.Subtask{},
		&models.ActivityLog{},
		&models.Comment{},
	)
	if err != nil {
		return fmt.Errorf("error en la migración: %w", err)
	}
	return nil
}

// SeedAdmin crea el administrador inicial si todavía no existe.
func SeedAdmin(db *gorm.DB, email, password string) error {
	var count int64
	if err := db.Model(&models.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return fmt.Errorf("error al buscar admin: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("error al generar hash de la contraseña del admin: %w", err)
	}
	admin := models.User{
		Name:         "Administrador",
		Email:        email,
		PasswordHash: string(hash),
		Role:         models.RoleAdmin,
		Active:       true,
	}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("error al crear admin: %w", err)
	}
	// guardar en log solo para verificar que si se creo el admin de la base de datos, comentarlo al pasar a produccion...
	//log.Printf("admin inicial creado: %s", email)
	return nil
}
