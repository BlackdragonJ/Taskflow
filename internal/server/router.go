package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"taskflow/internal/auth"
	"taskflow/internal/config"
	"taskflow/internal/httpx"
	"taskflow/internal/projects"
	"taskflow/internal/tasks"
	"taskflow/internal/users"
)

// NewRouter arma todas las rutas. webDir es la carpeta del frontend.
func NewRouter(db *gorm.DB, cfg config.Config, webDir string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), httpx.Recovery())

	tm := auth.NewTokenManager(cfg.JWTSecret, cfg.TokenTTL)
	access := projects.NewAccess(db)
	projectSvc := projects.NewService(db, access)
	taskSvc := tasks.NewService(db, access)

	authH := auth.NewHandler(auth.NewService(db, tm, cfg.DevMode))
	userH := users.NewHandler(db)
	projectH := projects.NewHandler(projectSvc)
	taskH := tasks.NewHandler(taskSvc, projectSvc)

	api := r.Group("/api")
	{
		api.POST("/auth/register", httpx.H(authH.Register))
		api.POST("/auth/login", httpx.H(authH.Login))
		api.POST("/auth/forgot", httpx.H(authH.Forgot))
		api.POST("/auth/reset", httpx.H(authH.Reset))
	}

	priv := api.Group("", auth.RequireAuth(db, tm))
	{
		priv.GET("/auth/me", httpx.H(authH.Me))
		priv.GET("/dashboard", httpx.H(projectH.Dashboard))

		priv.GET("/projects", httpx.H(projectH.List))
		priv.GET("/projects/:id", httpx.H(projectH.Get))

		priv.GET("/boards/:boardId", httpx.H(taskH.Board))
		priv.POST("/boards/:boardId/tasks", httpx.H(taskH.Create))

		priv.GET("/tasks/:id", httpx.H(taskH.Get))
		priv.PATCH("/tasks/:id", httpx.H(taskH.Update))
		priv.POST("/tasks/:id/move", httpx.H(taskH.Move))

		priv.POST("/tasks/:id/subtasks", httpx.H(taskH.AddSubtask))
		priv.PATCH("/tasks/:id/subtasks/:subId", httpx.H(taskH.UpdateSubtask))
		priv.DELETE("/tasks/:id/subtasks/:subId", httpx.H(taskH.DeleteSubtask))

		priv.GET("/tasks/:id/comments", httpx.H(taskH.ListComments))
		priv.POST("/tasks/:id/comments", httpx.H(taskH.AddComment))
	}

	admin := priv.Group("", auth.RequireAdmin())
	{
		admin.GET("/users", httpx.H(userH.List))
		admin.PATCH("/users/:id", httpx.H(userH.Update))

		admin.POST("/projects", httpx.H(projectH.Create))
		admin.PUT("/projects/:id", httpx.H(projectH.Update))
		admin.DELETE("/projects/:id", httpx.H(projectH.Delete))
		admin.POST("/projects/:id/members", httpx.H(projectH.AddMember))
		admin.DELETE("/projects/:id/members/:userId", httpx.H(projectH.RemoveMember))
		admin.POST("/projects/:id/boards", httpx.H(projectH.CreateBoard))
		admin.DELETE("/boards/:boardId", httpx.H(projectH.DeleteBoard))

		admin.POST("/tasks/:id/rollback", httpx.H(taskH.Rollback))
		admin.DELETE("/tasks/:id", httpx.H(taskH.Delete))
	}

	// frontend
	r.Static("/assets", webDir+"/assets")
	pages := map[string]string{
		"/":          "login.html",
		"/register":  "register.html",
		"/forgot":    "forgot.html",
		"/dashboard": "dashboard.html",
		"/board":     "board.html",
		"/users":     "users.html",
	}
	for route, file := range pages {
		r.StaticFile(route, webDir+"/pages/"+file)
	}

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": httpx.NotFound("Recurso")})
			return
		}
		c.Redirect(http.StatusFound, "/")
	})
	return r
}
