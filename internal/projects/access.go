package projects

import (
	"errors"

	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

// Access concentra las reglas de "quién puede ver qué". Lo usan también
// otros paquetes (tasks) para no duplicar consultas de membresía.
type Access struct {
	db *gorm.DB
}

func NewAccess(db *gorm.DB) *Access {
	return &Access{db: db}
}

func (a *Access) IsMember(projectID, userID uint) (bool, error) {
	var count int64
	err := a.db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Count(&count).Error
	return count > 0, err
}

// CheckProject: el admin ve todo, el miembro solo sus proyectos asignados.
func (a *Access) CheckProject(user *models.User, projectID uint) error {
	if user == nil {
		return httpx.Unauthorized("Sesión no iniciada")
	}
	var count int64
	if err := a.db.Model(&models.Project{}).Where("id = ?", projectID).Count(&count).Error; err != nil {
		return httpx.Internal(err)
	}
	if count == 0 {
		return httpx.NotFound("Proyecto")
	}
	if user.IsAdmin() {
		return nil
	}
	ok, err := a.IsMember(projectID, user.ID)
	if err != nil {
		return httpx.Internal(err)
	}
	if !ok {
		return httpx.Forbidden("No perteneces a este proyecto")
	}
	return nil
}

// CheckBoard valida acceso y devuelve el project_id del tablero.
func (a *Access) CheckBoard(user *models.User, boardID uint) (uint, error) {
	var board models.Board
	if err := a.db.Select("id", "project_id").First(&board, boardID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, httpx.NotFound("Tablero")
		}
		return 0, httpx.Internal(err)
	}
	return board.ProjectID, a.CheckProject(user, board.ProjectID)
}

// ProjectMembers se usa para llenar el select de responsable.
func (a *Access) ProjectMembers(projectID uint) ([]models.User, error) {
	var users []models.User
	err := a.db.Joins("JOIN project_members pm ON pm.user_id = users.id").
		Where("pm.project_id = ? AND users.active = ?", projectID, true).
		Order("users.name").Find(&users).Error
	return users, err
}
