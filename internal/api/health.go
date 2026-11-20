package api

import "net/http"

type healthHandler struct{}

func (h *healthHandler) Health(_ http.ResponseWriter, _ *http.Request) Response {
	return NewResponse(http.StatusOK)
}
