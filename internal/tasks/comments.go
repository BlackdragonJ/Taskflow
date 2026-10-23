package tasks

import (
	"strings"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

// ListComments devuelve los comentarios en orden cronológico. Con afterID>0
// solo trae los nuevos (lo usa el polling del modal).
func (s *Service) ListComments(user *models.User, taskID, afterID uint) ([]models.Comment, error) {
	if _, err := s.load(s.db, user, taskID); err != nil {
		return nil, err
	}
	var list []models.Comment
	q := s.db.Preload("User").Where("task_id = ?", taskID)
	if afterID > 0 {
		q = q.Where("id > ?", afterID)
	}
	if err := q.Order("id").Find(&list).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	return list, nil
}

func (s *Service) AddComment(user *models.User, taskID uint, body string) (*models.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, httpx.Validation(map[string]string{"body": "El comentario no puede estar vacío"})
	}
	if len([]rune(body)) > 2000 {
		return nil, httpx.Validation(map[string]string{"body": "Máximo 2000 caracteres"})
	}
	if _, err := s.load(s.db, user, taskID); err != nil {
		return nil, err
	}
	c := models.Comment{TaskID: taskID, UserID: user.ID, Body: body}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	c.User = user
	return &c, nil
}
