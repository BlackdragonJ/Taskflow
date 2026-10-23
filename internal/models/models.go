package models

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Status string

// Orden del flujo Kanban. El índice se usa para saber si un movimiento es hacia atrás.
const (
	StatusBacklog    Status = "backlog"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusDone       Status = "done"
)

var StatusFlow = []Status{StatusBacklog, StatusInProgress, StatusReview, StatusDone}

var StatusLabels = map[Status]string{
	StatusBacklog:    "Por hacer",
	StatusInProgress: "En progreso",
	StatusReview:     "En revisión / QA",
	StatusDone:       "Completado",
}

func (s Status) Valid() bool {
	_, ok := StatusLabels[s]
	return ok
}

func (s Status) Index() int {
	for i, st := range StatusFlow {
		if st == s {
			return i
		}
	}
	return -1
}

type Priority string

const (
	PriorityHigh   Priority = "alta"
	PriorityMedium Priority = "media"
	PriorityLow    Priority = "baja"
)

func (p Priority) Valid() bool {
	return p == PriorityHigh || p == PriorityMedium || p == PriorityLow
}

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	Email        string    `gorm:"size:150;not null;uniqueIndex" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         Role      `gorm:"size:20;not null;default:member" json:"role"`
	Active       bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}

type PasswordReset struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	User      User      `gorm:"constraint:OnDelete:CASCADE"`
	TokenHash string    `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

type Project struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:120;not null" json:"name"`
	Description string    `gorm:"size:500" json:"description"`
	OwnerID     uint      `gorm:"not null" json:"owner_id"`
	Owner       *User     `gorm:"constraint:OnDelete:RESTRICT" json:"owner,omitempty"`
	Members     []User    `gorm:"many2many:project_members;constraint:OnDelete:CASCADE" json:"members,omitempty"`
	Boards      []Board   `gorm:"constraint:OnDelete:CASCADE" json:"boards,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Board struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"not null;index" json:"project_id"`
	Name      string    `gorm:"size:120;not null" json:"name"`
	Columns   []Column  `gorm:"constraint:OnDelete:CASCADE" json:"columns,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Cada tablero tiene exactamente una columna por estado.
type Column struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	BoardID  uint   `gorm:"not null;uniqueIndex:idx_board_status" json:"board_id"`
	Name     string `gorm:"size:60;not null" json:"name"`
	Status   Status `gorm:"size:20;not null;uniqueIndex:idx_board_status" json:"status"`
	Position int    `gorm:"not null" json:"position"`
	Tasks    []Task `gorm:"constraint:OnDelete:CASCADE" json:"tasks,omitempty"`
}

type Task struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ColumnID    uint       `gorm:"not null;index" json:"column_id"`
	Title       string     `gorm:"size:150;not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Priority    Priority   `gorm:"size:10;not null;default:media" json:"priority"`
	Labels      string     `gorm:"size:200" json:"labels"`
	AssigneeID  *uint      `gorm:"index" json:"assignee_id"`
	Assignee    *User      `gorm:"constraint:OnDelete:SET NULL" json:"assignee,omitempty"`
	CreatedByID uint       `gorm:"not null" json:"created_by_id"`
	CreatedBy   *User      `gorm:"constraint:OnDelete:RESTRICT" json:"created_by,omitempty"`
	StartDate   *time.Time `json:"start_date"`
	DueDate     *time.Time `json:"due_date"`
	ClosedAt    *time.Time `json:"closed_at"`
	Subtasks    []Subtask  `gorm:"constraint:OnDelete:CASCADE" json:"subtasks,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Subtask struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"not null;index" json:"task_id"`
	Title     string    `gorm:"size:150;not null" json:"title"`
	Done      bool      `gorm:"not null;default:false" json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivityLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TaskID     uint      `gorm:"not null;index" json:"task_id"`
	Task       *Task     `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	UserID     uint      `gorm:"not null" json:"user_id"`
	User       *User     `gorm:"constraint:OnDelete:RESTRICT" json:"user,omitempty"`
	Action     string    `gorm:"size:40;not null" json:"action"`
	FromStatus Status    `gorm:"size:20" json:"from_status"`
	ToStatus   Status    `gorm:"size:20" json:"to_status"`
	Detail     string    `gorm:"size:500" json:"detail"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

type Comment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"not null;index" json:"task_id"`
	Task      *Task     `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	User      *User     `gorm:"constraint:OnDelete:RESTRICT" json:"user,omitempty"`
	Body      string    `gorm:"size:2000;not null" json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
