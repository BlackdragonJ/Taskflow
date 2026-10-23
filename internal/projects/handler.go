package projects

import (
	"github.com/gin-gonic/gin"

	"taskflow/internal/auth"
	"taskflow/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(c *gin.Context) error {
	list, err := h.svc.List(auth.CurrentUser(c))
	if err != nil {
		return err
	}
	return httpx.OK(c, list)
}

func (h *Handler) Get(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	p, err := h.svc.Get(auth.CurrentUser(c), id)
	if err != nil {
		return err
	}
	return httpx.OK(c, p)
}

func (h *Handler) Create(c *gin.Context) error {
	var in ProjectInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.Create(auth.CurrentUser(c), in)
	if err != nil {
		return err
	}
	return httpx.Created(c, p)
}

func (h *Handler) Update(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in ProjectInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.svc.Update(auth.CurrentUser(c), id, in)
	if err != nil {
		return err
	}
	return httpx.OK(c, p)
}

func (h *Handler) Delete(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(id); err != nil {
		return err
	}
	return httpx.NoContent(c)
}

func (h *Handler) AddMember(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		UserID uint `json:"user_id"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.UserID == 0 {
		return httpx.Validation(map[string]string{"user_id": "Selecciona un usuario"})
	}
	if err := h.svc.AddMember(id, in.UserID); err != nil {
		return err
	}
	return h.Get(c)
}

func (h *Handler) RemoveMember(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	userID, err := httpx.ParamID(c, "userId")
	if err != nil {
		return err
	}
	if err := h.svc.RemoveMember(id, userID); err != nil {
		return err
	}
	return httpx.NoContent(c)
}

func (h *Handler) CreateBoard(c *gin.Context) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	b, err := h.svc.CreateBoard(id, in.Name)
	if err != nil {
		return err
	}
	return httpx.Created(c, b)
}

func (h *Handler) DeleteBoard(c *gin.Context) error {
	id, err := httpx.ParamID(c, "boardId")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteBoard(id); err != nil {
		return err
	}
	return httpx.NoContent(c)
}

func (h *Handler) Dashboard(c *gin.Context) error {
	stats, err := h.svc.Dashboard(auth.CurrentUser(c))
	if err != nil {
		return err
	}
	return httpx.OK(c, stats)
}
