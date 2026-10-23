package auth

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

const ctxUserKey = "currentUser"

// RequireAuth valida el JWT y carga el usuario desde la BD. Se consulta la BD
// en cada request para que un cambio de rol o una desactivación aplique de inmediato.
func RequireAuth(db *gorm.DB, tm *TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		raw, found := strings.CutPrefix(header, "Bearer ")
		if !found || raw == "" {
			httpx.Fail(c, httpx.Unauthorized("Sesión no iniciada"))
			return
		}
		userID, err := tm.Parse(raw)
		if err != nil {
			httpx.Fail(c, httpx.Unauthorized("Tu sesión expiró, inicia sesión de nuevo"))
			return
		}

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				httpx.Fail(c, httpx.Unauthorized("Usuario no encontrado"))
				return
			}
			httpx.Fail(c, httpx.Internal(err))
			return
		}
		if !user.Active {
			httpx.Fail(c, httpx.Forbidden("Tu cuenta está desactivada"))
			return
		}
		c.Set(ctxUserKey, &user)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !CurrentUser(c).IsAdmin() {
			httpx.Fail(c, httpx.Forbidden("Esta acción requiere rol de Administrador"))
			return
		}
		c.Next()
	}
}

// CurrentUser puede devolver nil si la ruta no pasó por RequireAuth.
func CurrentUser(c *gin.Context) *models.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}
