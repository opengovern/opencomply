package tasks

import (
	"crypto/rsa"
	"net/http"

	api2 "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/opencomply/services/task/db"
	"github.com/opengovern/opencomply/services/task/api"


	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// var (
// 	//go:embed email/invite.html
// 	inviteEmailTemplate string
// )

type httpRoutes struct {
	logger *zap.Logger

	platformPrivateKey *rsa.PrivateKey
	db                 db.Database
}

func (r *httpRoutes) Register(e *echo.Echo) {
	v1 := e.Group("/api/v1")
	// Get all tasks
	v1.GET("/tasks", httpserver.AuthorizeHandler(r.getTasks,api2.EditorRole))
	// Create a new task
	v1.POST("/tasks", httpserver.AuthorizeHandler(r.createTask, api2.EditorRole))
	// Get Task Result
	v1.GET("/tasks/:id/result", httpserver.AuthorizeHandler(r.getTaskResult, api2.EditorRole))

	

}

func bindValidate(ctx echo.Context, i interface{}) error {
	if err := ctx.Bind(i); err != nil {
		return err
	}

	if err := ctx.Validate(i); err != nil {
		return err
	}

	return nil
}


func (r *httpRoutes) getTasks(ctx echo.Context) error {
	tasks, err := r.db.GetTaskList()
	if err != nil {
		r.logger.Error("failed to get tasks", zap.Error(err))
		return ctx.JSON(http.StatusInternalServerError, "failed to get tasks")

	}

	return ctx.JSON(http.StatusOK, tasks)
}

func (r *httpRoutes) createTask(ctx echo.Context) error {
	var task api.TaskCreateRequest
	if err := bindValidate(ctx, &task); err != nil {
		r.logger.Error("failed to bind task", zap.Error(err))
		return ctx.JSON(http.StatusBadRequest, "failed to bind task")
	}
	newTask := db.Task{
		Name: task.Name,
		Description: task.Description,
		ImageUrl: task.ImageUrl,
		Interval: task.Interval,
		AutoRun: task.AutoRun,
	}

	if err := r.db.CreateTask(&newTask); err != nil {
		r.logger.Error("failed to create task", zap.Error(err))
		return ctx.JSON(http.StatusInternalServerError, "failed to create task")
	}

	return ctx.JSON(http.StatusCreated, task)
}

func (r *httpRoutes) getTaskResult(ctx echo.Context) error {
	id := ctx.Param("id")
	taskResults, err := r.db.GetTaskResult(id)
	if err != nil {
		r.logger.Error("failed to get task result", zap.Error(err))
		return ctx.JSON(http.StatusInternalServerError, "failed to get task result")
	}

	return ctx.JSON(http.StatusOK, taskResults)


}
