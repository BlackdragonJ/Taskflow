package users

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"taskflow/internal/auth"
	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

// Gestión de usuarios: solo la usa el administrador (ver router).
type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) List(c *gin.Context) error {
	var users []models.User
	if err := h.db.Order("name").Find(&users).Error; err != nil {
		return httpx.Internal(err)
	}
	return httpx.OK(c, users)
}

func (h *Handler) Update(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Role   *models.Role `json:"role"`
		Active *bool        `json:"active"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}

	me := auth.CurrentUser(c)
	if me == nil {
		return httpx.Unauthorized("Sesión no iniciada")
	}
	// evita que el admin se quite el acceso a sí mismo por accidente
	if me.ID == id {
		if in.Role != nil && *in.Role != models.RoleAdmin {
			return httpx.BadRequest("No puedes quitarte el rol de administrador")
		}
		if in.Active != nil && !*in.Active {
			return httpx.BadRequest("No puedes desactivar tu propia cuenta")
		}
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.NotFound("Usuario")
		}
		return httpx.Internal(err)
	}

	updates := map[string]any{}
	if in.Role != nil {
		if *in.Role != models.RoleAdmin && *in.Role != models.RoleMember {
			return httpx.Validation(map[string]string{"role": "Rol inválido"})
		}
		updates["role"] = *in.Role
	}
	if in.Active != nil {
		updates["active"] = *in.Active
	}
	if len(updates) == 0 {
		return httpx.BadRequest("No hay cambios que aplicar")
	}
	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		return httpx.Internal(err)
	}
	return httpx.OK(c, user)
}
