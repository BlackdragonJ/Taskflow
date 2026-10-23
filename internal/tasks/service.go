package tasks

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
	"taskflow/internal/projects"
)

const dateLayout = "2006-01-02"

type Service struct {
	db     *gorm.DB
	access *projects.Access
	now    func() time.Time // reemplazable en tests
}

func NewService(db *gorm.DB, access *projects.Access) *Service {
	return &Service{db: db, access: access, now: time.Now}
}

type Progress struct {
	Done    int `json:"done"`
	Total   int `json:"total"`
	Percent int `json:"percent"`
}

func calcProgress(subs []models.Subtask) Progress {
	p := Progress{Total: len(subs)}
	for _, s := range subs {
		if s.Done {
			p.Done++
		}
	}
	if p.Total > 0 {
		p.Percent = p.Done * 100 / p.Total
	}
	return p
}

type TaskView struct {
	models.Task
	Status      models.Status `json:"status"`
	StatusLabel string        `json:"status_label"`
	BoardID     uint          `json:"board_id"`
	Progress    Progress      `json:"progress"`
	Overdue     bool          `json:"overdue"`
}

type TaskDetail struct {
	TaskView
	Moves     []MoveOption         `json:"moves"`
	Rollback  []MoveOption         `json:"rollback"`
	Members   []models.User        `json:"members"`
	Activity  []models.ActivityLog `json:"activity"`
	CanEdit   bool                 `json:"can_edit"`
	CanDelete bool                 `json:"can_delete"`
}

// taskCtx agrupa la tarea con su columna y proyecto, que casi todas las
// operaciones necesitan para validar permisos y estado.
type taskCtx struct {
	task      models.Task
	column    models.Column
	projectID uint
}

func (tc *taskCtx) state() TaskState {
	p := calcProgress(tc.task.Subtasks)
	return TaskState{
		Status:        tc.column.Status,
		AssigneeID:    tc.task.AssigneeID,
		SubtasksTotal: p.Total,
		SubtasksDone:  p.Done,
	}
}

func (s *Service) load(db *gorm.DB, user *models.User, taskID uint) (*taskCtx, error) {
	tc := &taskCtx{}
	err := db.Preload("Assignee").Preload("CreatedBy").
		Preload("Subtasks", func(db *gorm.DB) *gorm.DB { return db.Order("id") }).
		First(&tc.task, taskID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.NotFound("Tarea")
	}
	if err != nil {
		return nil, httpx.Internal(err)
	}
	if err := db.First(&tc.column, tc.task.ColumnID).Error; err != nil {
		return nil, httpx.Internal(fmt.Errorf("columna de tarea %d: %w", taskID, err))
	}
	tc.projectID, err = s.access.CheckBoard(user, tc.column.BoardID)
	if err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *Service) view(t models.Task, col models.Column) TaskView {
	v := TaskView{
		Task:        t,
		Status:      col.Status,
		StatusLabel: models.StatusLabels[col.Status],
		BoardID:     col.BoardID,
		Progress:    calcProgress(t.Subtasks),
	}
	if t.DueDate != nil && col.Status != models.StatusDone {
		v.Overdue = t.DueDate.Format(dateLayout) < s.now().Format(dateLayout)
	}
	return v
}

// Las tareas completadas quedan cerradas; solo el admin puede tocarlas.
func canEdit(tc *taskCtx, user *models.User) bool {
	return tc.column.Status != models.StatusDone || user.IsAdmin()
}

func (s *Service) log(db *gorm.DB, taskID, userID uint, action string, from, to models.Status, detail string) error {
	entry := models.ActivityLog{
		TaskID: taskID, UserID: userID, Action: action,
		FromStatus: from, ToStatus: to, Detail: truncate(detail, 500),
	}
	return db.Create(&entry).Error
}

// ---------- consultas ----------

func (s *Service) Detail(user *models.User, taskID uint) (*TaskDetail, error) {
	tc, err := s.load(s.db, user, taskID)
	if err != nil {
		return nil, err
	}
	d := &TaskDetail{
		TaskView:  s.view(tc.task, tc.column),
		Moves:     AvailableMoves(tc.state(), user),
		Rollback:  RollbackTargets(tc.column.Status, user),
		CanEdit:   canEdit(tc, user),
		CanDelete: user.IsAdmin(),
	}
	if d.Members, err = s.access.ProjectMembers(tc.projectID); err != nil {
		return nil, httpx.Internal(err)
	}
	err = s.db.Preload("User").Where("task_id = ?", taskID).
		Order("created_at DESC, id DESC").Find(&d.Activity).Error
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return d, nil
}

type ColumnCards struct {
	models.Column
	Cards []TaskView `json:"cards"`
}

func (s *Service) BoardCards(columns []models.Column) ([]ColumnCards, error) {
	out := make([]ColumnCards, 0, len(columns))
	for _, col := range columns {
		var list []models.Task
		err := s.db.Preload("Assignee").Preload("Subtasks").
			Where("column_id = ?", col.ID).
			Order(gorm.Expr("CASE priority WHEN 'alta' THEN 0 WHEN 'media' THEN 1 ELSE 2 END")).
			Order("due_date IS NULL, due_date").Order("id").
			Find(&list).Error
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cc := ColumnCards{Column: col, Cards: make([]TaskView, 0, len(list))}
		for _, t := range list {
			cc.Cards = append(cc.Cards, s.view(t, col))
		}
		out = append(out, cc)
	}
	return out, nil
}

// ---------- creación y edición ----------

type CreateInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Labels      string `json:"labels"`
	AssigneeID  *uint  `json:"assignee_id"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
}

func (s *Service) Create(user *models.User, boardID uint, in CreateInput) (*TaskView, error) {
	projectID, err := s.access.CheckBoard(user, boardID)
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		fields["title"] = "El título es obligatorio"
	} else if len(title) > 150 {
		fields["title"] = "Máximo 150 caracteres"
	}
	prio := models.Priority(strings.TrimSpace(in.Priority))
	if prio == "" {
		prio = models.PriorityMedium
	}
	if !prio.Valid() {
		fields["priority"] = "Prioridad inválida"
	}
	labels, labelErr := normalizeLabels(in.Labels)
	if labelErr != "" {
		fields["labels"] = labelErr
	}
	start, err := parseDate(in.StartDate)
	if err != nil {
		fields["start_date"] = "Fecha inválida"
	}
	due, err := parseDate(in.DueDate)
	if err != nil {
		fields["due_date"] = "Fecha inválida"
	}
	if start != nil && due != nil && due.Before(*start) {
		fields["due_date"] = "La fecha límite no puede ser anterior a la de inicio"
	}
	if in.AssigneeID != nil && *in.AssigneeID == 0 {
		in.AssigneeID = nil
	}
	if in.AssigneeID != nil {
		if msg, err := s.checkAssignee(projectID, *in.AssigneeID); err != nil {
			return nil, err
		} else if msg != "" {
			fields["assignee_id"] = msg
		}
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}

	var backlog models.Column
	err = s.db.Where("board_id = ? AND status = ?", boardID, models.StatusBacklog).First(&backlog).Error
	if err != nil {
		return nil, httpx.Internal(fmt.Errorf("tablero %d sin columna backlog: %w", boardID, err))
	}

	task := models.Task{
		ColumnID:    backlog.ID,
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Priority:    prio,
		Labels:      labels,
		AssigneeID:  in.AssigneeID,
		CreatedByID: user.ID,
		StartDate:   start,
		DueDate:     due,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		return s.log(tx, task.ID, user.ID, "created", "", models.StatusBacklog, "Tarea creada: "+title)
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	if err := s.db.Preload("Assignee").First(&task, task.ID).Error; err != nil {
		return nil, httpx.Internal(err)
	}
	v := s.view(task, backlog)
	return &v, nil
}

// checkAssignee devuelve un mensaje de validación si el usuario no puede ser responsable.
func (s *Service) checkAssignee(projectID, userID uint) (string, error) {
	ok, err := s.access.IsMember(projectID, userID)
	if err != nil {
		return "", httpx.Internal(err)
	}
	if !ok {
		return "El responsable debe ser miembro del proyecto", nil
	}
	var u models.User
	if err := s.db.Select("id", "active").First(&u, userID).Error; err != nil {
		return "", httpx.Internal(err)
	}
	if !u.Active {
		return "El usuario está desactivado", nil
	}
	return "", nil
}

// UpdateInput usa punteros para distinguir "no enviado" de "vacío".
// assignee_id = 0 quita el responsable; fechas "" las limpian.
type UpdateInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Priority    *string `json:"priority"`
	Labels      *string `json:"labels"`
	AssigneeID  *uint   `json:"assignee_id"`
	StartDate   *string `json:"start_date"`
	DueDate     *string `json:"due_date"`
}

func (s *Service) Update(user *models.User, taskID uint, in UpdateInput) (*TaskDetail, error) {
	tc, err := s.load(s.db, user, taskID)
	if err != nil {
		return nil, err
	}
	if !canEdit(tc, user) {
		return nil, httpx.Forbidden("La tarea está completada y cerrada; solo un Administrador puede editarla")
	}

	t := tc.task
	fields := map[string]string{}
	updates := map[string]any{}
	var changed []string

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		switch {
		case title == "":
			fields["title"] = "El título es obligatorio"
		case len(title) > 150:
			fields["title"] = "Máximo 150 caracteres"
		case title != t.Title:
			updates["title"] = title
			changed = append(changed, "título")
		}
	}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		if len(desc) > 5000 {
			fields["description"] = "Máximo 5000 caracteres"
		} else if desc != t.Description {
			updates["description"] = desc
			changed = append(changed, "descripción")
		}
	}
	if in.Priority != nil {
		p := models.Priority(*in.Priority)
		if !p.Valid() {
			fields["priority"] = "Prioridad inválida"
		} else if p != t.Priority {
			updates["priority"] = p
			changed = append(changed, "prioridad → "+string(p))
		}
	}
	if in.Labels != nil {
		labels, msg := normalizeLabels(*in.Labels)
		if msg != "" {
			fields["labels"] = msg
		} else if labels != t.Labels {
			updates["labels"] = labels
			changed = append(changed, "etiquetas")
		}
	}

	start, due := t.StartDate, t.DueDate
	if in.StartDate != nil {
		if start, err = parseDate(*in.StartDate); err != nil {
			fields["start_date"] = "Fecha inválida"
		} else {
			updates["start_date"] = start
			changed = append(changed, "fecha de inicio")
		}
	}
	if in.DueDate != nil {
		if due, err = parseDate(*in.DueDate); err != nil {
			fields["due_date"] = "Fecha inválida"
		} else {
			updates["due_date"] = due
			changed = append(changed, "fecha límite")
		}
	}
	if start != nil && due != nil && due.Before(*start) {
		fields["due_date"] = "La fecha límite no puede ser anterior a la de inicio"
	}

	var assigneeMsg string
	if in.AssigneeID != nil {
		newID := *in.AssigneeID
		switch {
		case newID == 0 && t.AssigneeID != nil:
			// una tarea iniciada no puede quedarse sin responsable
			if tc.column.Status != models.StatusBacklog {
				fields["assignee_id"] = "Una tarea iniciada debe tener responsable; reasígnala a otra persona"
			} else {
				updates["assignee_id"] = nil
				assigneeMsg = "Responsable removido"
			}
		case newID != 0 && (t.AssigneeID == nil || *t.AssigneeID != newID):
			msg, err := s.checkAssignee(tc.projectID, newID)
			if err != nil {
				return nil, err
			}
			if msg != "" {
				fields["assignee_id"] = msg
			} else {
				updates["assignee_id"] = newID
				var u models.User
				if err := s.db.Select("name").First(&u, newID).Error; err != nil {
					return nil, httpx.Internal(err)
				}
				assigneeMsg = "Responsable: " + u.Name
			}
		}
	}

	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}
	if len(updates) == 0 {
		return s.Detail(user, taskID)
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Task{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
			return err
		}
		if len(changed) > 0 {
			if err := s.log(tx, taskID, user.ID, "updated", "", "", "Editó: "+strings.Join(changed, ", ")); err != nil {
				return err
			}
		}
		if assigneeMsg != "" {
			return s.log(tx, taskID, user.ID, "assigned", "", "", assigneeMsg)
		}
		return nil
	})
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return s.Detail(user, taskID)
}

func (s *Service) Delete(user *models.User, taskID uint) error {
	if _, err := s.load(s.db, user, taskID); err != nil {
		return err
	}
	if err := s.db.Delete(&models.Task{}, taskID).Error; err != nil {
		return httpx.Internal(err)
	}
	return nil
}

// ---------- transiciones ----------

func (s *Service) Move(user *models.User, taskID uint, to models.Status, note string) (*TaskDetail, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		tc, err := s.load(tx, user, taskID)
		if err != nil {
			return err
		}
		if err := CheckMove(tc.state(), to, user, note); err != nil {
			return err
		}
		action := "moved"
		if isRejection(tc.column.Status, to) {
			action = "rejected"
		}
		return s.changeStatus(tx, tc, user, to, action, strings.TrimSpace(note))
	})
	if err != nil {
		return nil, httpx.AsAppError(err)
	}
	return s.Detail(user, taskID)
}

func (s *Service) Rollback(user *models.User, taskID uint, to models.Status, reason string) (*TaskDetail, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		tc, err := s.load(tx, user, taskID)
		if err != nil {
			return err
		}
		if err := CheckRollback(tc.column.Status, to, user, reason); err != nil {
			return err
		}
		return s.changeStatus(tx, tc, user, to, "rollback", strings.TrimSpace(reason))
	})
	if err != nil {
		return nil, httpx.AsAppError(err)
	}
	return s.Detail(user, taskID)
}

func (s *Service) changeStatus(tx *gorm.DB, tc *taskCtx, user *models.User, to models.Status, action, note string) error {
	var target models.Column
	err := tx.Where("board_id = ? AND status = ?", tc.column.BoardID, to).First(&target).Error
	if err != nil {
		return httpx.Internal(fmt.Errorf("columna %s del tablero %d: %w", to, tc.column.BoardID, err))
	}

	updates := map[string]any{"column_id": target.ID}
	if to == models.StatusDone {
		now := s.now()
		updates["closed_at"] = &now
	} else if tc.task.ClosedAt != nil {
		updates["closed_at"] = nil // reabierta
	}
	if err := tx.Model(&models.Task{}).Where("id = ?", tc.task.ID).Updates(updates).Error; err != nil {
		return httpx.Internal(err)
	}

	detail := models.StatusLabels[tc.column.Status] + " → " + models.StatusLabels[to]
	if note != "" {
		detail += ". " + note
	}
	if err := s.log(tx, tc.task.ID, user.ID, action, tc.column.Status, to, detail); err != nil {
		return httpx.Internal(err)
	}

	// el defecto queda también como comentario para que el responsable lo vea
	if action == "rejected" {
		c := models.Comment{TaskID: tc.task.ID, UserID: user.ID, Body: "🔁 Devuelta por QA: " + note}
		if err := tx.Create(&c).Error; err != nil {
			return httpx.Internal(err)
		}
	}
	return nil
}

// ---------- helpers ----------

func parseDate(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	// se guarda como medianoche UTC, solo importa el día
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func normalizeLabels(raw string) (string, string) {
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.Split(raw, ",") {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] {
			continue
		}
		if len(l) > 30 {
			return "", "Cada etiqueta puede tener máximo 30 caracteres"
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) > 8 {
		return "", "Máximo 8 etiquetas"
	}
	return strings.Join(out, ","), ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
