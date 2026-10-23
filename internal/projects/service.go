package projects

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

type Service struct {
	db     *gorm.DB
	access *Access
}

func NewService(db *gorm.DB, access *Access) *Service {
	return &Service{db: db, access: access}
}

type ProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *ProjectInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "El nombre es obligatorio"
	} else if len(in.Name) > 120 {
		fields["name"] = "Máximo 120 caracteres"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Máximo 500 caracteres"
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return nil
}

func (s *Service) List(user *models.User) ([]models.Project, error) {
	var list []models.Project
	q := s.db.Preload("Boards").Order("created_at DESC")
	if !user.IsAdmin() {
		q = q.Joins("JOIN project_members pm ON pm.project_id = projects.id").
			Where("pm.user_id = ?", user.ID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	return list, nil
}

func (s *Service) Get(user *models.User, id uint) (*models.Project, error) {
	if err := s.access.CheckProject(user, id); err != nil {
		return nil, err
	}
	var p models.Project
	err := s.db.Preload("Boards").
		Preload("Members", func(db *gorm.DB) *gorm.DB { return db.Order("name") }).
		First(&p, id).Error
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return &p, nil
}

// Create crea el proyecto con un tablero inicial y agrega al dueño como miembro.
func (s *Service) Create(owner *models.User, in ProjectInput) (*models.Project, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	p := &models.Project{Name: in.Name, Description: in.Description, OwnerID: owner.ID}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		if err := tx.Model(p).Association("Members").Append(owner); err != nil {
			return err
		}
		_, err := createBoard(tx, p.ID, "Tablero principal")
		return err
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.Get(owner, p.ID)
}

func (s *Service) Update(user *models.User, id uint, in ProjectInput) (*models.Project, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if err := s.access.CheckProject(user, id); err != nil {
		return nil, err
	}
	err := s.db.Model(&models.Project{}).Where("id = ?", id).
		Updates(map[string]any{"name": in.Name, "description": in.Description}).Error
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.Get(user, id)
}

func (s *Service) Delete(id uint) error {
	res := s.db.Delete(&models.Project{}, id)
	if res.Error != nil {
		return httpx.Internal(res.Error)
	}
	if res.RowsAffected == 0 {
		return httpx.NotFound("Proyecto")
	}
	return nil
}

func (s *Service) AddMember(projectID, userID uint) error {
	var p models.Project
	if err := s.db.First(&p, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.NotFound("Proyecto")
		}
		return httpx.Internal(err)
	}
	var u models.User
	if err := s.db.First(&u, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.NotFound("Usuario")
		}
		return httpx.Internal(err)
	}
	if !u.Active {
		return httpx.BadRequest("No se puede agregar un usuario desactivado")
	}
	already, err := s.access.IsMember(projectID, userID)
	if err != nil {
		return httpx.Internal(err)
	}
	if already {
		return httpx.Conflict("El usuario ya es miembro del proyecto")
	}
	if err := s.db.Model(&p).Association("Members").Append(&u); err != nil {
		return httpx.Internal(err)
	}
	return nil
}

func (s *Service) RemoveMember(projectID, userID uint) error {
	var p models.Project
	if err := s.db.First(&p, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.NotFound("Proyecto")
		}
		return httpx.Internal(err)
	}
	if p.OwnerID == userID {
		return httpx.BadRequest("No se puede quitar al creador del proyecto")
	}

	// si tiene tareas abiertas en el proyecto hay que reasignarlas primero
	var open int64
	err := s.db.Model(&models.Task{}).
		Joins("JOIN columns ON columns.id = tasks.column_id").
		Joins("JOIN boards ON boards.id = columns.board_id").
		Where("boards.project_id = ? AND tasks.assignee_id = ? AND columns.status <> ?",
			projectID, userID, models.StatusDone).
		Count(&open).Error
	if err != nil {
		return httpx.Internal(err)
	}
	if open > 0 {
		return httpx.Conflict("El usuario tiene tareas abiertas asignadas en este proyecto, reasígnalas primero")
	}

	if err := s.db.Model(&p).Association("Members").Delete(&models.User{ID: userID}); err != nil {
		return httpx.Internal(err)
	}
	return nil
}

// ---- tableros ----

func createBoard(tx *gorm.DB, projectID uint, name string) (*models.Board, error) {
	board := &models.Board{ProjectID: projectID, Name: name}
	for i, st := range models.StatusFlow {
		board.Columns = append(board.Columns, models.Column{
			Name: models.StatusLabels[st], Status: st, Position: i,
		})
	}
	if err := tx.Create(board).Error; err != nil {
		return nil, err
	}
	return board, nil
}

func (s *Service) CreateBoard(projectID uint, name string) (*models.Board, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return nil, httpx.Validation(map[string]string{"name": "El nombre es obligatorio (máx. 120)"})
	}
	var count int64
	if err := s.db.Model(&models.Project{}).Where("id = ?", projectID).Count(&count).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	if count == 0 {
		return nil, httpx.NotFound("Proyecto")
	}
	board, err := createBoard(s.db, projectID, name)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return board, nil
}

func (s *Service) DeleteBoard(boardID uint) error {
	res := s.db.Delete(&models.Board{}, boardID)
	if res.Error != nil {
		return httpx.Internal(res.Error)
	}
	if res.RowsAffected == 0 {
		return httpx.NotFound("Tablero")
	}
	return nil
}

type BoardView struct {
	models.Board
	ProjectName string        `json:"project_name"`
	Members     []models.User `json:"members"`
}

// GetBoard devuelve el tablero con columnas y tareas. Las tarjetas se arman en
// el paquete tasks (ver tasks.Service.BoardCards) para no duplicar lógica.
func (s *Service) GetBoard(user *models.User, boardID uint) (*BoardView, error) {
	projectID, err := s.access.CheckBoard(user, boardID)
	if err != nil {
		return nil, err
	}
	var view BoardView
	err = s.db.Preload("Columns", func(db *gorm.DB) *gorm.DB { return db.Order("position") }).
		First(&view.Board, boardID).Error
	if err != nil {
		return nil, httpx.Internal(err)
	}
	var p models.Project
	if err := s.db.Select("id", "name").First(&p, projectID).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	view.ProjectName = p.Name
	if view.Members, err = s.access.ProjectMembers(projectID); err != nil {
		return nil, httpx.Internal(err)
	}
	return &view, nil
}

// ---- dashboard ----

type Stats struct {
	ProjectID   uint   `json:"project_id"`
	ProjectName string `json:"project_name"`
	Total       int64  `json:"total"`
	Backlog     int64  `json:"backlog"`
	InProgress  int64  `json:"in_progress"`
	Review      int64  `json:"review"`
	Done        int64  `json:"done"`
	Overdue     int64  `json:"overdue"`
}

func (s *Service) Dashboard(user *models.User) ([]Stats, error) {
	projects, err := s.List(user)
	if err != nil {
		return nil, err
	}
	out := make([]Stats, 0, len(projects))
	for _, p := range projects {
		st, err := s.projectStats(p.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		st.ProjectName = p.Name
		out = append(out, st)
	}
	return out, nil
}

func (s *Service) projectStats(projectID uint) (Stats, error) {
	st := Stats{ProjectID: projectID}

	var rows []struct {
		Status models.Status
		N      int64
	}
	err := s.db.Model(&models.Task{}).
		Select("columns.status AS status, COUNT(*) AS n").
		Joins("JOIN columns ON columns.id = tasks.column_id").
		Joins("JOIN boards ON boards.id = columns.board_id").
		Where("boards.project_id = ?", projectID).
		Group("columns.status").Scan(&rows).Error
	if err != nil {
		return st, err
	}
	for _, r := range rows {
		st.Total += r.N
		switch r.Status {
		case models.StatusBacklog:
			st.Backlog = r.N
		case models.StatusInProgress:
			st.InProgress = r.N
		case models.StatusReview:
			st.Review = r.N
		case models.StatusDone:
			st.Done = r.N
		}
	}

	// vencida = fecha límite antes de hoy y no completada
	today := time.Now().Format("2006-01-02")
	todayUTC, _ := time.Parse("2006-01-02", today)
	err = s.db.Model(&models.Task{}).
		Joins("JOIN columns ON columns.id = tasks.column_id").
		Joins("JOIN boards ON boards.id = columns.board_id").
		Where("boards.project_id = ? AND columns.status <> ? AND tasks.due_date IS NOT NULL AND tasks.due_date < ?",
			projectID, models.StatusDone, todayUTC).
		Count(&st.Overdue).Error
	return st, err
}
