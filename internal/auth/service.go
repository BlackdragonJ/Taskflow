package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

const resetTTL = 30 * time.Minute

type Service struct {
	db      *gorm.DB
	tokens  *TokenManager
	devMode bool
}

func NewService(db *gorm.DB, tm *TokenManager, devMode bool) *Service {
	return &Service{db: db, tokens: tm, devMode: devMode}
}

type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) Register(in RegisterInput) (*models.User, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = normalizeEmail(in.Email)

	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "El nombre es obligatorio"
	} else if len(in.Name) > 100 {
		fields["name"] = "El nombre no puede pasar de 100 caracteres"
	}
	if in.Email == "" {
		fields["email"] = "El correo es obligatorio"
	} else if !ValidEmail(in.Email) {
		fields["email"] = "El formato del correo no es válido"
	}
	if in.Password == "" {
		fields["password"] = "La contraseña es obligatoria"
	} else if msg := passwordError(in.Password); msg != "" {
		fields["password"] = msg
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}

	var count int64
	if err := s.db.Model(&models.User{}).Where("email = ?", in.Email).Count(&count).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	if count > 0 {
		return nil, httpx.Validation(map[string]string{"email": "Ya existe una cuenta con ese correo"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	user := &models.User{
		Name:         in.Name,
		Email:        in.Email,
		PasswordHash: string(hash),
		Role:         models.RoleMember, // el admin promueve después si hace falta
		Active:       true,
	}
	if err := s.db.Create(user).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	return user, nil
}

func (s *Service) Login(email, password string) (string, *models.User, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return "", nil, httpx.Validation(map[string]string{
			"email":    "Correo y contraseña son obligatorios",
			"password": "Correo y contraseña son obligatorios",
		})
	}

	// mismo mensaje para correo inexistente y contraseña mala
	badCreds := httpx.Unauthorized("Correo o contraseña incorrectos")

	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, badCreds
		}
		return "", nil, httpx.Internal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, badCreds
	}
	if !user.Active {
		return "", nil, httpx.Forbidden("Tu cuenta está desactivada, contacta al administrador")
	}

	token, err := s.tokens.Generate(&user)
	if err != nil {
		return "", nil, httpx.Internal(err)
	}
	return token, &user, nil
}

// RequestReset genera un token de un solo uso. Responde igual exista o no el
// correo para no revelar qué cuentas están registradas.
func (s *Service) RequestReset(email string) (devToken string, err error) {
	email = normalizeEmail(email)
	if !ValidEmail(email) {
		return "", httpx.Validation(map[string]string{"email": "El formato del correo no es válido"})
	}

	var user models.User
	if err := s.db.Where("email = ? AND active = ?", email, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", httpx.Internal(err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", httpx.Internal(err)
	}
	token := hex.EncodeToString(buf)

	reset := models.PasswordReset{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(resetTTL),
	}
	if err := s.db.Create(&reset).Error; err != nil {
		return "", httpx.Internal(err)
	}

	// TODO: enviar por correo cuando tengamos SMTP configurado
	log.Printf("token de recuperación para %s: %s", email, token)
	if s.devMode {
		return token, nil
	}
	return "", nil
}

func (s *Service) ResetPassword(token, newPassword string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return httpx.Validation(map[string]string{"token": "El código de recuperación es obligatorio"})
	}
	if msg := passwordError(newPassword); msg != "" {
		return httpx.Validation(map[string]string{"password": msg})
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var reset models.PasswordReset
		err := tx.Where("token_hash = ?", hashToken(token)).First(&reset).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.BadRequest("El código no es válido")
		}
		if err != nil {
			return httpx.Internal(err)
		}
		if reset.UsedAt != nil || time.Now().After(reset.ExpiresAt) {
			return httpx.BadRequest("El código ya fue usado o expiró, solicita uno nuevo")
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return httpx.Internal(err)
		}
		if err := tx.Model(&models.User{}).Where("id = ?", reset.UserID).
			Update("password_hash", string(hash)).Error; err != nil {
			return httpx.Internal(err)
		}
		now := time.Now()
		if err := tx.Model(&reset).Update("used_at", &now).Error; err != nil {
			return httpx.Internal(err)
		}
		return nil
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
