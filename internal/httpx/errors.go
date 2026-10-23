package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError es lo único que llega al cliente. Cualquier otro error se loguea
// y se responde como 500 genérico para no filtrar detalles internos.
type AppError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Err     error             `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func BadRequest(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func Validation(fields map[string]string) *AppError {
	return &AppError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_error",
		Message: "Revisa los campos marcados",
		Fields:  fields,
	}
}

func Unauthorized(msg string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: msg}
}

func Forbidden(msg string) *AppError {
	return &AppError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func NotFound(what string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: "not_found", Message: what + " no encontrado"}
}

func Conflict(msg string) *AppError {
	return &AppError{Status: http.StatusConflict, Code: "conflict", Message: msg}
}

// InvalidTransition se usa cuando el flujo Kanban no permite el movimiento.
func InvalidTransition(msg string) *AppError {
	return &AppError{Status: http.StatusConflict, Code: "invalid_transition", Message: msg}
}

func Internal(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Code: "internal_error",
		Message: "Ocurrió un error inesperado, intenta de nuevo", Err: err}
}

func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return Internal(err)
}
