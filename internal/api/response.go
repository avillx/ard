package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"ard/internal/types"
)

type Response interface {
	StatusCode() int
}

type response struct {
	statusCode int
}

func NewResponse(code int) response {
	return response{
		statusCode: code,
	}
}

func (r response) StatusCode() int {
	return r.statusCode
}

type JSONResponse interface {
	Response
	Content() any
}

type jsonResponse struct {
	response

	content any
}

func NewJSONResponse[T string | any](code int, msg T) *jsonResponse {
	var s any
	s = msg

	if stringMessage, ok := any(msg).(string); ok {
		s = map[string]any{
			"message": stringMessage,
		}
	}

	return &jsonResponse{
		response: NewResponse(code),
		content:  s,
	}
}

func (r *jsonResponse) Content() any {
	return r.content
}

type internalError struct {
	response

	cause error
}

func NewInternalError(cause error) Response {
	return &internalError{
		response: NewResponse(http.StatusInternalServerError),
		cause:    cause,
	}
}

func (e *internalError) Err() error {
	return e.cause
}

func NewBadRequest[T string | map[string]any](msg T) Response {
	return NewJSONResponse(http.StatusBadRequest, msg)
}

func NewInvalidRequest(err error) Response {
	var r map[string]any

	if problems := types.ResovleValidationProblems(err); len(problems) > 0 {
		r = map[string]any{
			"problems": problems,
		}
	}

	if r == nil {
		r = map[string]any{
			"problems": err.Error(),
		}
	}

	return NewJSONResponse(http.StatusBadRequest, r)
}

func NewNotFound(cause string) Response {
	if cause != "" {
		return NewJSONResponse(http.StatusNotFound, cause)
	}
	return NewResponse(http.StatusNotFound)
}

func decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("broken json")
	}

	if validator, ok := any(v).(types.Validator); ok {
		if err := validator.Validate(r.Context()); err != nil {
			return v, err
		}
	}

	return v, nil
}

type Stream struct {
	w             http.ResponseWriter
	mu            sync.Mutex
	commitHeaders sync.Once
}

func newStream(w http.ResponseWriter) *Stream {
	return &Stream{w: w}
}

func (s *Stream) send(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commitHeaders.Do(func() {
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
		s.w.Header().Set("Connection", "keep-alive")
		s.w.WriteHeader(http.StatusOK)
	})

	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "data: %s\n\n", data)
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Stream) sendError(status int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commitHeaders.Do(func() {
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
		s.w.Header().Set("Connection", "keep-alive")
		s.w.WriteHeader(status)
	})
	data, err := json.Marshal(map[string]any{
		"error": err.Error(),
	})
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "error: %s\n\n", data)
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Stream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Fprint(s.w, "data: [DONE]")
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}

type ErrorDTO struct {
	Message string `json:"msg"`
}
