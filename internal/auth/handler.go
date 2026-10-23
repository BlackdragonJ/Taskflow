package auth

import (
	"github.com/gin-gonic/gin"

	"taskflow/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(c *gin.Context) error {
	var in RegisterInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	user, err := h.svc.Register(in)
	if err != nil {
		return err
	}
	return httpx.Created(c, user)
}

func (h *Handler) Login(c *gin.Context) error {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	token, user, err := h.svc.Login(in.Email, in.Password)
	if err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"token": token, "user": user})
}

func (h *Handler) Forgot(c *gin.Context) error {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	devToken, err := h.svc.RequestReset(in.Email)
	if err != nil {
		return err
	}
	resp := gin.H{"message": "Si el correo está registrado, recibirás un código de recuperación"}
	if devToken != "" {
		resp["dev_token"] = devToken
	}
	return httpx.OK(c, resp)
}

func (h *Handler) Reset(c *gin.Context) error {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.ResetPassword(in.Token, in.Password); err != nil {
		return err
	}
	return httpx.OK(c, gin.H{"message": "Contraseña actualizada, ya puedes iniciar sesión"})
}

func (h *Handler) Me(c *gin.Context) error {
	u := CurrentUser(c)
	if u == nil {
		return httpx.Unauthorized("Sesión no iniciada")
	}
	return httpx.OK(c, u)
}
