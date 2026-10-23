package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"taskflow/internal/config"
	"taskflow/internal/database"
)

type apiClient struct {
	t      *testing.T
	router *gin.Engine
	token  string
}

type apiResp struct {
	Code  int
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func (c *apiClient) do(method, path string, body any) apiResp {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			c.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/api"+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)

	var out apiResp
	out.Code = rec.Code
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			c.t.Fatalf("%s %s: respuesta no es JSON: %s", method, path, rec.Body.String())
		}
	}
	return out
}

// must falla el test si el status no es el esperado.
func (c *apiClient) must(want int, method, path string, body any) apiResp {
	c.t.Helper()
	r := c.do(method, path, body)
	if r.Code != want {
		c.t.Fatalf("%s %s: status %d, want %d (%s: %s %v)", method, path, r.Code, want,
			r.Error.Code, r.Error.Message, r.Error.Fields)
	}
	return r
}

func decode[T any](t *testing.T, r apiResp) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.Data, &v); err != nil {
		t.Fatalf("decode: %v (%s)", err, r.Data)
	}
	return v
}

func setup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := database.SeedAdmin(db, "admin@test.com", "Admin123!"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{JWTSecret: "test", TokenTTL: time.Hour, DevMode: true}
	return NewRouter(db, cfg, "../../web")
}

func login(t *testing.T, r *gin.Engine, email, pw string) *apiClient {
	t.Helper()
	c := &apiClient{t: t, router: r}
	res := c.must(200, "POST", "/auth/login", map[string]string{"email": email, "password": pw})
	c.token = decode[struct{ Token string }](t, res).Token
	return c
}

type taskDetail struct {
	ID         uint    `json:"id"`
	Status     string  `json:"status"`
	AssigneeID *uint   `json:"assignee_id"`
	ClosedAt   *string `json:"closed_at"`
	Overdue    bool    `json:"overdue"`
	Progress   struct{ Done, Total, Percent int }
	Moves      []struct {
		Status  string `json:"status"`
		Kind    string `json:"kind"`
		Blocked string `json:"blocked"`
	} `json:"moves"`
	Rollback []struct{ Status string } `json:"rollback"`
	Activity []struct {
		Action string `json:"action"`
	} `json:"activity"`
	CanEdit bool `json:"can_edit"`
}

func TestAuthValidation(t *testing.T) {
	r := setup(t)
	c := &apiClient{t: t, router: r}

	res := c.must(422, "POST", "/auth/register", map[string]string{"name": "", "email": "malo", "password": "123"})
	for _, f := range []string{"name", "email", "password"} {
		if res.Error.Fields[f] == "" {
			t.Errorf("falta error de validación para %s", f)
		}
	}

	c.must(201, "POST", "/auth/register", map[string]string{"name": "Ana", "email": "ANA@test.com", "password": "Segura#2026"})
	res = c.must(422, "POST", "/auth/register", map[string]string{"name": "Ana 2", "email": "ana@test.com", "password": "Segura#2026"})
	if res.Error.Fields["email"] == "" {
		t.Error("correo duplicado (case-insensitive) debería fallar")
	}

	c.must(401, "POST", "/auth/login", map[string]string{"email": "ana@test.com", "password": "otra"})
	c.must(401, "GET", "/projects", nil)
	c.must(400, "POST", "/auth/login", "no-es-objeto")

	// recuperación de contraseña
	res = c.must(200, "POST", "/auth/forgot", map[string]string{"email": "ana@test.com"})
	tok := decode[struct {
		DevToken string `json:"dev_token"`
	}](t, res).DevToken
	if tok == "" {
		t.Fatal("en modo dev debería venir dev_token")
	}
	c.must(422, "POST", "/auth/reset", map[string]string{"token": tok, "password": "debil"})
	c.must(200, "POST", "/auth/reset", map[string]string{"token": tok, "password": "Nueva#2026"})
	c.must(400, "POST", "/auth/reset", map[string]string{"token": tok, "password": "Otra#20260"}) // ya usado
	login(t, r, "ana@test.com", "Nueva#2026")

	// correo inexistente responde igual (no revela cuentas)
	res = c.must(200, "POST", "/auth/forgot", map[string]string{"email": "nadie@test.com"})
	if decode[map[string]any](t, res)["dev_token"] != nil {
		t.Error("no debería generar token para correo inexistente")
	}
}

func TestKanbanFlow(t *testing.T) {
	r := setup(t)
	anon := &apiClient{t: t, router: r}
	anon.must(201, "POST", "/auth/register", map[string]string{"name": "Ana Dev", "email": "ana@test.com", "password": "Segura#2026"})
	anon.must(201, "POST", "/auth/register", map[string]string{"name": "Luis QA", "email": "luis@test.com", "password": "Segura#2026"})
	anon.must(201, "POST", "/auth/register", map[string]string{"name": "Externo", "email": "ext@test.com", "password": "Segura#2026"})

	adm := login(t, r, "admin@test.com", "Admin123!")
	ana := login(t, r, "ana@test.com", "Segura#2026")
	luis := login(t, r, "luis@test.com", "Segura#2026")
	ext := login(t, r, "ext@test.com", "Segura#2026")

	// solo admin crea proyectos
	ana.must(403, "POST", "/projects", map[string]string{"name": "X"})
	proj := decode[struct {
		ID     uint
		Boards []struct{ ID uint }
	}](t, adm.must(201, "POST", "/projects", map[string]string{"name": "Sistema QA"}))
	if len(proj.Boards) != 1 {
		t.Fatalf("el proyecto debería crearse con un tablero, tiene %d", len(proj.Boards))
	}
	boardID := proj.Boards[0].ID

	users := decode[[]struct {
		ID    uint
		Email string
	}](t, adm.must(200, "GET", "/users", nil))
	ids := map[string]uint{}
	for _, u := range users {
		ids[u.Email] = u.ID
	}
	adm.must(200, "POST", fmt.Sprintf("/projects/%d/members", proj.ID), map[string]uint{"user_id": ids["ana@test.com"]})
	adm.must(200, "POST", fmt.Sprintf("/projects/%d/members", proj.ID), map[string]uint{"user_id": ids["luis@test.com"]})
	adm.must(409, "POST", fmt.Sprintf("/projects/%d/members", proj.ID), map[string]uint{"user_id": ids["luis@test.com"]})

	// alguien fuera del proyecto no ve el tablero
	ext.must(403, "GET", fmt.Sprintf("/boards/%d", boardID), nil)
	if list := decode[[]any](t, ext.must(200, "GET", "/projects", nil)); len(list) != 0 {
		t.Errorf("usuario externo ve %d proyectos", len(list))
	}

	// crear tarea: validaciones
	res := ana.must(422, "POST", fmt.Sprintf("/boards/%d/tasks", boardID), map[string]any{
		"title": "", "priority": "urgente", "start_date": "2026-05-10", "due_date": "2026-05-01",
		"assignee_id": ids["ext@test.com"],
	})
	for _, f := range []string{"title", "priority", "due_date", "assignee_id"} {
		if res.Error.Fields[f] == "" {
			t.Errorf("falta validación de %s", f)
		}
	}

	task := decode[taskDetail](t, ana.must(201, "POST", fmt.Sprintf("/boards/%d/tasks", boardID), map[string]any{
		"title": "Login con JWT", "priority": "alta", "due_date": "2020-01-01",
	}))
	if task.Status != "backlog" {
		t.Fatalf("tarea nueva en %s", task.Status)
	}
	taskURL := fmt.Sprintf("/tasks/%d", task.ID)

	// vencida porque la fecha límite ya pasó
	d := decode[taskDetail](t, ana.must(200, "GET", taskURL, nil))
	if !d.Overdue {
		t.Error("la tarea debería marcarse como vencida")
	}
	for _, m := range d.Moves {
		if m.Status == d.Status {
			t.Error("el estado actual aparece en las opciones del select")
		}
	}

	// backlog -> en progreso sin responsable: bloqueado
	ana.must(409, "POST", taskURL+"/move", map[string]string{"status": "in_progress"})
	// no se puede saltar
	ana.must(409, "POST", taskURL+"/move", map[string]string{"status": "review"})

	ana.must(200, "PATCH", taskURL, map[string]any{"assignee_id": ids["ana@test.com"]})
	ana.must(200, "POST", taskURL+"/move", map[string]string{"status": "in_progress"})

	// una tarea iniciada no puede quedar sin responsable
	ana.must(422, "PATCH", taskURL, map[string]any{"assignee_id": 0})

	// miembro no puede retroceder ni por move ni por rollback
	ana.must(403, "POST", taskURL+"/move", map[string]string{"status": "backlog"})
	ana.must(403, "POST", taskURL+"/rollback", map[string]string{"status": "backlog", "reason": "x"})

	// subtareas: progreso y bloqueo del paso a revisión
	sub := decode[struct {
		Subtasks []struct{ ID uint }
		Progress struct{ Percent int }
	}](t, ana.must(201, "POST", taskURL+"/subtasks", map[string]string{"title": "Endpoint /login"}))
	ana.must(201, "POST", taskURL+"/subtasks", map[string]string{"title": "Middleware"})
	ana.must(409, "POST", taskURL+"/move", map[string]string{"status": "review"})

	d = decode[taskDetail](t, ana.must(200, "GET", taskURL, nil))
	if d.Progress.Total != 2 || d.Progress.Percent != 0 {
		t.Errorf("progreso inicial %+v", d.Progress)
	}
	subURL := fmt.Sprintf("%s/subtasks/%d", taskURL, sub.Subtasks[0].ID)
	half := decode[struct{ Progress struct{ Percent int } }](t, ana.must(200, "PATCH", subURL, map[string]bool{"done": true}))
	if half.Progress.Percent != 50 {
		t.Errorf("progreso esperado 50%%, got %d", half.Progress.Percent)
	}
	d = decode[taskDetail](t, ana.must(200, "GET", taskURL, nil))
	ana.must(200, "PATCH", fmt.Sprintf("%s/subtasks/%d", taskURL, sub.Subtasks[0].ID+1), map[string]bool{"done": true})

	// solo el responsable marca el avance como completo
	luis.must(403, "POST", taskURL+"/move", map[string]string{"status": "review"})
	ana.must(200, "POST", taskURL+"/move", map[string]string{"status": "review"})

	// QA devuelve sin nota → error; con nota → OK y queda comentario
	luis.must(422, "POST", taskURL+"/move", map[string]string{"status": "in_progress"})
	luis.must(200, "POST", taskURL+"/move", map[string]string{"status": "in_progress", "note": "No valida token expirado"})
	comments := decode[[]struct{ Body string }](t, ana.must(200, "GET", taskURL+"/comments", nil))
	if len(comments) != 1 {
		t.Errorf("el rechazo debería dejar un comentario, hay %d", len(comments))
	}

	ana.must(200, "POST", taskURL+"/move", map[string]string{"status": "review"})
	d = decode[taskDetail](t, luis.must(200, "POST", taskURL+"/move", map[string]string{"status": "done"}))
	if d.Status != "done" || d.ClosedAt == nil || d.Overdue {
		t.Errorf("tarea completada: status=%s closed=%v overdue=%v", d.Status, d.ClosedAt, d.Overdue)
	}
	if d.CanEdit {
		t.Error("una tarea completada no debería ser editable por un miembro")
	}
	ana.must(403, "PATCH", taskURL, map[string]string{"title": "cambio"})

	// comentarios siguen permitidos
	c := decode[struct{ ID uint }](t, ana.must(201, "POST", taskURL+"/comments", map[string]string{"body": "Listo en prod"}))
	newer := decode[[]any](t, ana.must(200, "GET", fmt.Sprintf("%s/comments?after=%d", taskURL, c.ID-1), nil))
	if len(newer) != 1 {
		t.Errorf("after= debería traer solo el último comentario, trae %d", len(newer))
	}
	ana.must(422, "POST", taskURL+"/comments", map[string]string{"body": "   "})

	// admin reabre con motivo
	adm.must(422, "POST", taskURL+"/rollback", map[string]string{"status": "in_progress"})
	adm.must(409, "POST", taskURL+"/rollback", map[string]string{"status": "done", "reason": "x"})
	d = decode[taskDetail](t, adm.must(200, "POST", taskURL+"/rollback", map[string]string{"status": "in_progress", "reason": "Bug en producción"}))
	if d.Status != "in_progress" || d.ClosedAt != nil {
		t.Errorf("tras rollback: status=%s closed=%v", d.Status, d.ClosedAt)
	}

	// bitácora: creación, movimientos, rechazo y rollback
	actions := map[string]int{}
	for _, a := range d.Activity {
		actions[a.Action]++
	}
	if actions["created"] != 1 || actions["moved"] != 4 || actions["rejected"] != 1 || actions["rollback"] != 1 {
		t.Errorf("bitácora inesperada: %v", actions)
	}

	// dashboard
	stats := decode[[]struct {
		Total, Done, InProgress, Overdue int
	}](t, adm.must(200, "GET", "/dashboard", nil))
	if len(stats) != 1 || stats[0].Total != 1 || stats[0].Overdue != 1 {
		t.Errorf("dashboard: %+v", stats)
	}

	// no se puede quitar a un miembro con tareas abiertas
	adm.must(409, "DELETE", fmt.Sprintf("/projects/%d/members/%d", proj.ID, ids["ana@test.com"]), nil)

	// solo admin elimina
	ana.must(403, "DELETE", taskURL, nil)
	adm.must(204, "DELETE", taskURL, nil)
	adm.must(404, "GET", taskURL, nil)
}

func TestAdminSelfProtection(t *testing.T) {
	r := setup(t)
	adm := login(t, r, "admin@test.com", "Admin123!")
	me := decode[struct{ ID uint }](t, adm.must(200, "GET", "/auth/me", nil))
	adm.must(400, "PATCH", fmt.Sprintf("/users/%d", me.ID), map[string]string{"role": "member"})
	adm.must(400, "PATCH", fmt.Sprintf("/users/%d", me.ID), map[string]bool{"active": false})
	adm.must(404, "PATCH", "/users/9999", map[string]bool{"active": false})
	adm.must(400, "GET", "/tasks/abc", nil)
}

func TestDeactivatedUserLosesAccess(t *testing.T) {
	r := setup(t)
	anon := &apiClient{t: t, router: r}
	anon.must(201, "POST", "/auth/register", map[string]string{"name": "Ana", "email": "ana@test.com", "password": "Segura#2026"})
	ana := login(t, r, "ana@test.com", "Segura#2026")
	adm := login(t, r, "admin@test.com", "Admin123!")

	u := decode[struct{ ID uint }](t, ana.must(200, "GET", "/auth/me", nil))
	adm.must(200, "PATCH", fmt.Sprintf("/users/%d", u.ID), map[string]bool{"active": false})

	// el token sigue siendo válido pero la cuenta ya no
	ana.must(403, "GET", "/projects", nil)
	anon.must(403, "POST", "/auth/login", map[string]string{"email": "ana@test.com", "password": "Segura#2026"})
}
