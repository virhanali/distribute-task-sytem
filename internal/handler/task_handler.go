package handler

import (
	"net/http"

	"distribute-task-sytem/internal/domain"
	"distribute-task-sytem/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type TaskHandler struct {
	service *service.TaskService
	logger  zerolog.Logger
}

func NewTaskHandler(service *service.TaskService, logger zerolog.Logger) *TaskHandler {
	return &TaskHandler{
		service: service,
		logger:  logger,
	}
}

// CreateTask godoc
// @Summary Create a new task
// @Description Create a new task with payload and priority
// @Tags tasks
// @Accept json
// @Produce json
// @Param task body domain.CreateTaskDTO true "Task Request"
// @Success 201 {object} SuccessResponse{data=domain.TaskResponse}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tasks [post]
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var dto domain.CreateTaskDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		h.logger.Debug().Err(err).Msg("invalid create task request body")
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body: "+err.Error()))
		return
	}

	task, err := h.service.Create(c.Request.Context(), dto)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to create task")
		c.JSON(http.StatusInternalServerError, NewErrorResponse("failed to create task"))
		return
	}

	c.JSON(http.StatusCreated, NewSuccessResponse(task.ToResponse()))
}

// CreateBulkTask godoc
// @Summary Create multiple tasks
// @Description Create multiple tasks in bulk
// @Tags tasks
// @Accept json
// @Produce json
// @Param tasks body domain.CreateBulkTaskDTO true "Bulk Task Request"
// @Success 201 {object} SuccessResponse{data=domain.BulkTaskResponse}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tasks/bulk [post]
func (h *TaskHandler) CreateBulkTask(c *gin.Context) {
	var dto domain.CreateBulkTaskDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body: "+err.Error()))
		return
	}

	resp, err := h.service.CreateBulk(c.Request.Context(), dto)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to create bulk tasks")
		c.JSON(http.StatusInternalServerError, NewErrorResponse("failed to process bulk request"))
		return
	}

	c.JSON(http.StatusCreated, NewSuccessResponse(resp))
}

// GetTask godoc
// @Summary Get task by ID
// @Description Get details of a specific task
// @Tags tasks
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} SuccessResponse{data=domain.TaskResponse}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tasks/{id} [get]
func (h *TaskHandler) GetTask(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid task id format"))
		return
	}

	task, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == domain.ErrTaskNotFound {
			c.JSON(http.StatusNotFound, NewErrorResponse("task not found"))
			return
		}
		h.logger.Error().Err(err).Str("task_id", idParam).Msg("failed to get task")
		c.JSON(http.StatusInternalServerError, NewErrorResponse("internal server error"))
		return
	}

	c.JSON(http.StatusOK, NewSuccessResponse(task.ToResponse()))
}

// CancelTask godoc
// @Summary Cancel a task
// @Description Cancel a pending or processing task
// @Tags tasks
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tasks/{id} [delete]
func (h *TaskHandler) CancelTask(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid task id format"))
		return
	}

	err = h.service.Cancel(c.Request.Context(), id)
	if err != nil {
		if err == domain.ErrTaskNotFound {
			c.JSON(http.StatusNotFound, NewErrorResponse("task not found"))
			return
		}
		if err == domain.ErrInvalidTaskStatus {
			c.JSON(http.StatusBadRequest, NewErrorResponse("task cannot be cancelled (already completed or cancelled)"))
			return
		}
		h.logger.Error().Err(err).Str("task_id", idParam).Msg("failed to cancel task")
		c.JSON(http.StatusInternalServerError, NewErrorResponse("internal server error"))
		return
	}

	c.JSON(http.StatusOK, NewSuccessResponse(nil))
}

// GetMetrics godoc
// @Summary Get task metrics
// @Description Get aggregated metrics of tasks
// @Tags metrics
// @Produce json
// @Success 200 {object} SuccessResponse{data=domain.TaskMetrics}
// @Failure 500 {object} ErrorResponse
// @Router /metrics [get]
func (h *TaskHandler) GetMetrics(c *gin.Context) {
	metrics, err := h.service.GetMetrics(c.Request.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to get metrics")
		c.JSON(http.StatusInternalServerError, NewErrorResponse("failed to get metrics"))
		return
	}

	c.JSON(http.StatusOK, NewSuccessResponse(metrics))
}
