package tasks

import (
	"net/http"
	"testing"

	"taskflow/internal/httpx"
	"taskflow/internal/models"
)

var (
	admin  = &models.User{ID: 1, Role: models.RoleAdmin}
	member = &models.User{ID: 2, Role: models.RoleMember}
	other  = &models.User{ID: 3, Role: models.RoleMember}
)

func uptr(v uint) *uint { return &v }

func statusOf(err error) int {
	if err == nil {
		return 0
	}
	return httpx.AsAppError(err).Status
}

func TestCheckMove(t *testing.T) {
	assigned := uptr(member.ID)

	cases := []struct {
		name  string
		state TaskState
		to    models.Status
		actor *models.User
		note  string
		want  int // 0 = permitido, si no el status HTTP esperado
	}{
		{"backlog a progreso sin responsable", TaskState{Status: models.StatusBacklog}, models.StatusInProgress, member, "", http.StatusConflict},
		{"backlog a progreso con responsable", TaskState{Status: models.StatusBacklog, AssigneeID: assigned}, models.StatusInProgress, other, "", 0},
		{"mismo estado", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusInProgress, member, "", http.StatusConflict},
		{"saltar de backlog a revisión", TaskState{Status: models.StatusBacklog, AssigneeID: assigned}, models.StatusReview, admin, "", http.StatusConflict},
		{"saltar de progreso a completado", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusDone, admin, "", http.StatusConflict},
		{"responsable envía a revisión", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusReview, member, "", 0},
		{"otro miembro no puede enviar a revisión", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusReview, other, "", http.StatusForbidden},
		{"admin puede enviar a revisión", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusReview, admin, "", 0},
		{"subtareas pendientes bloquean revisión", TaskState{Status: models.StatusInProgress, AssigneeID: assigned, SubtasksTotal: 3, SubtasksDone: 2}, models.StatusReview, member, "", http.StatusConflict},
		{"subtareas completas permiten revisión", TaskState{Status: models.StatusInProgress, AssigneeID: assigned, SubtasksTotal: 3, SubtasksDone: 3}, models.StatusReview, member, "", 0},
		{"QA aprueba", TaskState{Status: models.StatusReview, AssigneeID: assigned}, models.StatusDone, other, "", 0},
		{"QA rechaza sin nota", TaskState{Status: models.StatusReview, AssigneeID: assigned}, models.StatusInProgress, other, "  ", http.StatusUnprocessableEntity},
		{"QA rechaza con nota", TaskState{Status: models.StatusReview, AssigneeID: assigned}, models.StatusInProgress, other, "falla login", 0},
		{"miembro no retrocede a backlog", TaskState{Status: models.StatusInProgress, AssigneeID: assigned}, models.StatusBacklog, member, "", http.StatusForbidden},
		{"admin tampoco retrocede por move", TaskState{Status: models.StatusDone, AssigneeID: assigned}, models.StatusReview, admin, "", http.StatusForbidden},
		{"estado inválido", TaskState{Status: models.StatusBacklog}, models.Status("archivado"), admin, "", http.StatusBadRequest},
		{"sin usuario", TaskState{Status: models.StatusBacklog}, models.StatusInProgress, nil, "", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusOf(CheckMove(tc.state, tc.to, tc.actor, tc.note))
			if got != tc.want {
				t.Errorf("CheckMove() status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCheckRollback(t *testing.T) {
	cases := []struct {
		name     string
		from, to models.Status
		actor    *models.User
		reason   string
		want     int
	}{
		{"miembro no puede", models.StatusInProgress, models.StatusBacklog, member, "x", http.StatusForbidden},
		{"admin sin motivo", models.StatusInProgress, models.StatusBacklog, admin, "", http.StatusUnprocessableEntity},
		{"admin hacia adelante", models.StatusBacklog, models.StatusReview, admin, "x", http.StatusConflict},
		{"admin al mismo estado", models.StatusReview, models.StatusReview, admin, "x", http.StatusConflict},
		{"admin reabre completada", models.StatusDone, models.StatusInProgress, admin, "cliente reportó bug", 0},
		{"admin progreso a backlog", models.StatusInProgress, models.StatusBacklog, admin, "se repriorizó", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusOf(CheckRollback(tc.from, tc.to, tc.actor, tc.reason))
			if got != tc.want {
				t.Errorf("CheckRollback() status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAvailableMovesNeverIncludesCurrent(t *testing.T) {
	for _, st := range models.StatusFlow {
		for _, u := range []*models.User{admin, member} {
			opts := AvailableMoves(TaskState{Status: st, AssigneeID: uptr(member.ID)}, u)
			for _, o := range opts {
				if o.Status == st {
					t.Errorf("estado %s aparece en sus propias opciones (usuario %s)", st, u.Role)
				}
			}
		}
	}
}

func TestAvailableMoves(t *testing.T) {
	// sin responsable: el siguiente paso sale bloqueado con motivo
	opts := AvailableMoves(TaskState{Status: models.StatusBacklog}, member)
	if len(opts) != 1 || opts[0].Status != models.StatusInProgress || opts[0].Blocked == "" {
		t.Fatalf("backlog sin responsable: %+v", opts)
	}

	// en revisión: aprobar o devolver
	opts = AvailableMoves(TaskState{Status: models.StatusReview, AssigneeID: uptr(2)}, member)
	got := map[models.Status]MoveKind{}
	for _, o := range opts {
		if o.Blocked != "" {
			t.Errorf("opción %s no debería estar bloqueada: %s", o.Status, o.Blocked)
		}
		got[o.Status] = o.Kind
	}
	if got[models.StatusDone] != MoveForward || got[models.StatusInProgress] != MoveReject || len(got) != 2 {
		t.Errorf("revisión: opciones inesperadas %+v", got)
	}

	// completada: el miembro no tiene a dónde moverla
	if opts := AvailableMoves(TaskState{Status: models.StatusDone, AssigneeID: uptr(2)}, member); len(opts) != 0 {
		t.Errorf("completada no debería tener movimientos para miembro: %+v", opts)
	}
}

func TestRollbackTargets(t *testing.T) {
	if got := RollbackTargets(models.StatusReview, member); len(got) != 0 {
		t.Errorf("miembro no debería ver retrocesos: %+v", got)
	}
	got := RollbackTargets(models.StatusReview, admin)
	if len(got) != 2 || got[0].Status != models.StatusBacklog || got[1].Status != models.StatusInProgress {
		t.Errorf("admin desde revisión: %+v", got)
	}
	if got := RollbackTargets(models.StatusBacklog, admin); len(got) != 0 {
		t.Errorf("backlog no tiene estados anteriores: %+v", got)
	}
}

func TestCalcProgress(t *testing.T) {
	subs := []models.Subtask{{Done: true}, {Done: false}, {Done: true}}
	p := calcProgress(subs)
	if p.Done != 2 || p.Total != 3 || p.Percent != 66 {
		t.Errorf("progress = %+v", p)
	}
	if p := calcProgress(nil); p.Percent != 0 || p.Total != 0 {
		t.Errorf("sin subtareas = %+v", p)
	}
}

func TestNormalizeLabels(t *testing.T) {
	got, msg := normalizeLabels(" Frontend, bug,, FRONTEND ,docs ")
	if msg != "" || got != "frontend,bug,docs" {
		t.Errorf("normalizeLabels = %q, %q", got, msg)
	}
	if _, msg := normalizeLabels("a,b,c,d,e,f,g,h,i"); msg == "" {
		t.Error("debería rechazar más de 8 etiquetas")
	}
}
