package httpx

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type HandlerFunc func(c *gin.Context) error

// H adapta handlers que devuelven error al formato de Gin, así el manejo de
// errores queda en un solo lugar.
func H(fn HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			Fail(c, err)
		}
	}
}

func Fail(c *gin.Context, err error) {
	appErr := AsAppError(err)
	if appErr.Status >= 500 {
		log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, appErr)
	}
	c.AbortWithStatusJSON(appErr.Status, gin.H{"error": appErr})
}

func OK(c *gin.Context, data any) error {
	c.JSON(http.StatusOK, gin.H{"data": data})
	return nil
}

func Created(c *gin.Context, data any) error {
	c.JSON(http.StatusCreated, gin.H{"data": data})
	return nil
}

func NoContent(c *gin.Context) error {
	c.Status(http.StatusNoContent)
	return nil
}

// Bind parsea el JSON del body y responde 400 si viene mal formado.
func Bind(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return BadRequest("El cuerpo de la petición no es un JSON válido")
	}
	return nil
}

func ParamID(c *gin.Context, name string) (uint, error) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, BadRequest("Identificador inválido: " + name)
	}
	return uint(id), nil
}

// Recovery evita que un panic tumbe el servidor y responde en el mismo formato JSON.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, rec any) {
		log.Printf("[PANIC] %s %s: %v", c.Request.Method, c.Request.URL.Path, rec)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": &AppError{
			Code: "internal_error", Message: "Ocurrió un error inesperado, intenta de nuevo",
		}})
	})
}
