package api

import (
	"errors"
	"net/http"

	"ard/internal/task"
	"ard/internal/types"
)

// handler.
type taskHandler struct {
	taskSvc *task.Service
}

// GET /task.
func (s *taskHandler) List(_ http.ResponseWriter, _ *http.Request) Response {
	tasks, err := s.taskSvc.List()
	if err != nil {
		return NewInternalError(err)
	}

	return NewJSONResponse(http.StatusOK, tasks)
}

// POST /task/{name}.
func (s *taskHandler) Create(_ http.ResponseWriter, r *http.Request) Response {
	cfg, err := decode[task.Config](r)
	if err != nil {
		return NewInvalidRequest(err)
	}

	if err := s.taskSvc.Add(cfg); err != nil {
		if p := types.ResovleValidationProblems(err); len(p) > 0 {
			return NewInvalidRequest(err)
		}

		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

// PATCH /task/{name}.
func (s *taskHandler) Patch(_ http.ResponseWriter, r *http.Request) Response {
	taskName := r.PathValue("name")

	patch, err := decode[task.Patch](r)
	if err != nil {
		return NewInvalidRequest(err)
	}

	if err := s.taskSvc.Patch(taskName, patch); err != nil {
		if errors.Is(err, task.ErrIsNotExist) {
			return NewNotFound("task is not exist")
		}

		if p := types.ResovleValidationProblems(err); len(p) > 0 {
			return NewInvalidRequest(err)
		}

		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

// DELETE /task/{name}.
func (s *taskHandler) Delete(_ http.ResponseWriter, r *http.Request) Response {
	taskName := r.PathValue("name")
	if err := s.taskSvc.Delete(taskName); err != nil {
		if errors.Is(err, task.ErrIsNotExist) {
			return NewNotFound("task is not exist")
		}

		return NewInternalError(err)
	}
	return NewResponse(http.StatusOK)
}
