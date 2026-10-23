package tasks

import (
	"strings"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

// Reglas del flujo Kanban:
//
//	backlog -> in_progress   requiere responsable asignado
//	in_progress -> review    solo el responsable (o admin) y con subtareas completas
//	review -> done           aprobación de QA
//	review -> in_progress    rechazo de QA, requiere nota con el defecto
//
// Cualquier otro retroceso es un "rollback" exclusivo del administrador y
// va por un endpoint aparte. Saltarse estados no está permitido para nadie.

type MoveKind string

const (
	MoveForward MoveKind = "forward"
	MoveReject  MoveKind = "reject"
)

type MoveOption struct {
	Status  models.Status `json:"status"`
	Label   string        `json:"label"`
	Kind    MoveKind      `json:"kind"`
	Blocked string        `json:"blocked,omitempty"`
}

// TaskState es lo mínimo que necesitan las reglas; así se prueban sin BD.
type TaskState struct {
	Status        models.Status
	AssigneeID    *uint
	SubtasksTotal int
	SubtasksDone  int
}

func isRejection(from, to models.Status) bool {
	return from == models.StatusReview && to == models.StatusInProgress
}

// CheckMove valida un movimiento normal (no rollback).
func CheckMove(t TaskState, to models.Status, actor *models.User, note string) error {
	if actor == nil {
		return httpx.Unauthorized("Sesión no iniciada")
	}
	if !to.Valid() {
		return httpx.BadRequest("Estado destino inválido")
	}
	if t.Status == to {
		return httpx.InvalidTransition("La tarea ya se encuentra en " + models.StatusLabels[to])
	}

	from := t.Status
	diff := to.Index() - from.Index()

	if isRejection(from, to) {
		if strings.TrimSpace(note) == "" {
			return httpx.Validation(map[string]string{"note": "Describe el defecto encontrado para devolver la tarea"})
		}
		return nil
	}
	if diff < 0 {
		return httpx.Forbidden("Solo un Administrador puede retroceder una tarea (usa la acción Retroceder)")
	}
	if diff > 1 {
		return httpx.InvalidTransition("No se puede saltar estados: el siguiente paso es " +
			models.StatusLabels[models.StatusFlow[from.Index()+1]])
	}

	switch to {
	case models.StatusInProgress:
		if t.AssigneeID == nil {
			return httpx.InvalidTransition("Asigna un responsable antes de iniciar la tarea")
		}
	case models.StatusReview:
		if t.AssigneeID == nil {
			return httpx.InvalidTransition("La tarea no tiene responsable")
		}
		if *t.AssigneeID != actor.ID && !actor.IsAdmin() {
			return httpx.Forbidden("Solo el responsable puede marcar la tarea como lista para revisión")
		}
	}
	return nil
}

// CheckRollback valida el retroceso administrativo.
func CheckRollback(from, to models.Status, actor *models.User, reason string) error {
	if !actor.IsAdmin() {
		return httpx.Forbidden("Solo un Administrador puede retroceder una tarea")
	}
	if !to.Valid() {
		return httpx.BadRequest("Estado destino inválido")
	}
	if to.Index() >= from.Index() {
		return httpx.InvalidTransition("El retroceso solo puede ir a un estado anterior")
	}
	if strings.TrimSpace(reason) == "" {
		return httpx.Validation(map[string]string{"reason": "Indica el motivo del retroceso"})
	}
	return nil
}

// AvailableMoves arma las opciones del <select> de estado. Nunca incluye el
// estado actual. Solo lista lo que el usuario realmente puede hacer.
func AvailableMoves(t TaskState, actor *models.User) []MoveOption {
	opts := []MoveOption{}
	for _, st := range models.StatusFlow {
		if st == t.Status {
			continue
		}
		opt := MoveOption{Status: st, Label: models.StatusLabels[st], Kind: MoveForward}
		if isRejection(t.Status, st) {
			opt.Kind = MoveReject
			opt.Label += " (devolver por defectos)"
		}
		// la nota de rechazo se pide en el front, aquí solo importa si se puede
		if err := CheckMove(t, st, actor, "-"); err != nil {
			// el siguiente paso se muestra deshabilitado con el motivo; lo demás se oculta
			if st.Index() != t.Status.Index()+1 {
				continue
			}
			opt.Blocked = httpx.AsAppError(err).Message
		}
		opts = append(opts, opt)
	}
	return opts
}

// RollbackTargets: estados anteriores al actual, solo para admin.
func RollbackTargets(from models.Status, actor *models.User) []MoveOption {
	opts := []MoveOption{}
	if !actor.IsAdmin() {
		return opts
	}
	for _, st := range models.StatusFlow[:max(from.Index(), 0)] {
		opts = append(opts, MoveOption{Status: st, Label: models.StatusLabels[st], Kind: "rollback"})
	}
	return opts
}
