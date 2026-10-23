package tasks

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

type SubtaskResult struct {
	Subtasks []models.Subtask `json:"subtasks"`
	Progress Progress         `json:"progress"`
}

func (s *Service) subtaskResult(taskID uint) (*SubtaskResult, error) {
	var subs []models.Subtask
	if err := s.db.Where("task_id = ?", taskID).Order("id").Find(&subs).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	return &SubtaskResult{Subtasks: subs, Progress: calcProgress(subs)}, nil
}

func (s *Service) editableTask(user *models.User, taskID uint) (*taskCtx, error) {
	tc, err := s.load(s.db, user, taskID)
	if err != nil {
		return nil, err
	}
	if !canEdit(tc, user) {
		return nil, httpx.Forbidden("La tarea está completada y cerrada")
	}
	return tc, nil
}

func (s *Service) AddSubtask(user *models.User, taskID uint, title string) (*SubtaskResult, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 150 {
		return nil, httpx.Validation(map[string]string{"title": "El título es obligatorio (máx. 150)"})
	}
	if _, err := s.editableTask(user, taskID); err != nil {
		return nil, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.Subtask{TaskID: taskID, Title: title}).Error; err != nil {
			return err
		}
		return s.log(tx, taskID, user.ID, "subtask", "", "", "Agregó subtarea: "+title)
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.subtaskResult(taskID)
}

type SubtaskPatch struct {
	Title *string `json:"title"`
	Done  *bool   `json:"done"`
}

func (s *Service) UpdateSubtask(user *models.User, taskID, subID uint, in SubtaskPatch) (*SubtaskResult, error) {
	if _, err := s.editableTask(user, taskID); err != nil {
		return nil, err
	}
	sub, err := s.findSubtask(taskID, subID)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 150 {
			return nil, httpx.Validation(map[string]string{"title": "El título es obligatorio (máx. 150)"})
		}
		updates["title"] = title
	}
	if in.Done != nil {
		updates["done"] = *in.Done
	}
	if len(updates) == 0 {
		return nil, httpx.BadRequest("No hay cambios que aplicar")
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(sub).Updates(updates).Error; err != nil {
			return err
		}
		if in.Done != nil && *in.Done != sub.Done {
			verb := "Completó"
			if !*in.Done {
				verb = "Reabrió"
			}
			return s.log(tx, taskID, user.ID, "subtask", "", "", verb+" subtarea: "+sub.Title)
		}
		return nil
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.subtaskResult(taskID)
}

func (s *Service) DeleteSubtask(user *models.User, taskID, subID uint) (*SubtaskResult, error) {
	if _, err := s.editableTask(user, taskID); err != nil {
		return nil, err
	}
	sub, err := s.findSubtask(taskID, subID)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(sub).Error; err != nil {
			return err
		}
		return s.log(tx, taskID, user.ID, "subtask", "", "", "Eliminó subtarea: "+sub.Title)
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.subtaskResult(taskID)
}

// findSubtask también comprueba que la subtarea pertenezca a la tarea de la URL.
func (s *Service) findSubtask(taskID, subID uint) (*models.Subtask, error) {
	var sub models.Subtask
	err := s.db.Where("id = ? AND task_id = ?", subID, taskID).First(&sub).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.NotFound("Subtarea")
	}
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return &sub, nil
}
