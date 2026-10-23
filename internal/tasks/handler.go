package tasks

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"taskflow/internal/auth"
	"taskflow/internal/httpx"
	"taskflow/internal/models"
	"taskflow/internal/projects"
)

type Handler struct {
	svc      *Service
	projects *projects.Service
}

func NewHandler(svc *Service, ps *projects.Service) *Handler {
	return &Handler{svc: svc, projects: ps}
}

// Board devuelve el tablero completo con las tarjetas de cada columna.
func (h *Handler) Board(c *gin.Context) error {
	boardID, err := httpx.ParamID(c, "boardId")
	if err != nil {
		return err
	}
	user := auth.CurrentUser(c)
	board, err := h.projects.GetBoard(user, boardID)
	if err != nil {
		return err
	}
	cols, err := h.svc.BoardCards(board.Columns)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{
		"id":           board.ID,
		"name":         board.Name,
		"project_id":   board.ProjectID,
		"project_name": board.ProjectName,
		"members":      board.Members,
		"columns":      cols,
		"is_admin":     user.IsAdmin(),
	})
}

func (h *Handler) Create(c *gin.Context) error {
	boardID, err := httpx.ParamID(c, "boardId")
	if err != nil {
		return err
	}
	var in CreateInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	t, err := h.svc.Create(auth.CurrentUser(c), boardID, in)
	if err != nil {
		return err
	}
	return httpx.Created(c, t)
}

func (h *Handler) Get(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	d, err := h.svc.Detail(auth.CurrentUser(c), id)
	if err != nil {
		return err
	}
	return httpx.OK(c, d)
}

func (h *Handler) Update(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in UpdateInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.svc.Update(auth.CurrentUser(c), id, in)
	if err != nil {
		return err
	}
	return httpx.OK(c, d)
}

func (h *Handler) Delete(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(auth.CurrentUser(c), id); err != nil {
		return err
	}
	return httpx.NoContent(c)
}

type moveInput struct {
	Status models.Status `json:"status"`
	Note   string        `json:"note"`
}

func (h *Handler) Move(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in moveInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.svc.Move(auth.CurrentUser(c), id, in.Status, in.Note)
	if err != nil {
		return err
	}
	return httpx.OK(c, d)
}

func (h *Handler) Rollback(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status models.Status `json:"status"`
		Reason string        `json:"reason"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.svc.Rollback(auth.CurrentUser(c), id, in.Status, in.Reason)
	if err != nil {
		return err
	}
	return httpx.OK(c, d)
}

func (h *Handler) AddSubtask(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Title string `json:"title"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.AddSubtask(auth.CurrentUser(c), id, in.Title)
	if err != nil {
		return err
	}
	return httpx.Created(c, res)
}

func (h *Handler) UpdateSubtask(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	subID, err := httpx.ParamID(c, "subId")
	if err != nil {
		return err
	}
	var in SubtaskPatch
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	res, err := h.svc.UpdateSubtask(auth.CurrentUser(c), id, subID, in)
	if err != nil {
		return err
	}
	return httpx.OK(c, res)
}

func (h *Handler) DeleteSubtask(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	subID, err := httpx.ParamID(c, "subId")
	if err != nil {
		return err
	}
	res, err := h.svc.DeleteSubtask(auth.CurrentUser(c), id, subID)
	if err != nil {
		return err
	}
	return httpx.OK(c, res)
}

func (h *Handler) ListComments(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var after uint64
	if raw := c.Query("after"); raw != "" {
		if after, err = strconv.ParseUint(raw, 10, 64); err != nil {
			return httpx.BadRequest("Parámetro 'after' inválido")
		}
	}
	list, err := h.svc.ListComments(auth.CurrentUser(c), id, uint(after))
	if err != nil {
		return err
	}
	return httpx.OK(c, list)
}

func (h *Handler) AddComment(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	cm, err := h.svc.AddComment(auth.CurrentUser(c), id, in.Body)
	if err != nil {
		return err
	}
	return httpx.Created(c, cm)
}
