package api

import (
	"errors"
	"net/http"
	"time"

	"ard/internal/agent"
	"ard/internal/session"
	"ard/internal/types"
)

type HeaderDTO struct {
	ID           session.ID     `json:"session_id"`
	InputTokens  int64          `json:"input_tokens"`
	OutputTokens int64          `json:"output_tokens"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Extras       map[string]any `json:"extras,omitempty"`
	Cause        string         `json:"error,omitempty"`
}

type SessionDTO struct {
	HeaderDTO

	Messages []MessageDTO `json:"messages"`
}

type MessageDTO struct {
	Role      string              `json:"role"`
	Content   []agent.ContentPart `json:"content"`
	ToolCalls []ToolCallDTO       `json:"tool_calls,omitempty"`
}

func messagesToDTO(msgs []agent.Message) []MessageDTO {
	dtos := []MessageDTO{}
	for _, m := range msgs {
		dto := MessageDTO{
			Role:    string(m.Role()),
			Content: m.Content(),
		}
		if agentMessage, ok := m.(*agent.AgentMessage); ok {
			dto.ToolCalls = toolCallsToDTO(agentMessage.ToolCalls())
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

type sessionHandler struct {
	sessSvc *session.Service
}

func (h *sessionHandler) Get(_ http.ResponseWriter, r *http.Request) Response {
	agentID := agent.ID(r.PathValue("agent"))
	sessID := session.ID(r.PathValue("session"))

	sess, err := h.sessSvc.Session(agentID, sessID)
	if err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("session is not exist")
		}
		return NewInternalError(err)
	}

	dto := SessionDTO{
		HeaderDTO: sessHeaderToDTO(sess),
		Messages:  messagesToDTO(sess.Messages()),
	}

	return NewJSONResponse(http.StatusOK, dto)
}

func (h *sessionHandler) Sessions(_ http.ResponseWriter, r *http.Request) Response {
	agentID := agent.ID(r.PathValue("agent"))

	dtos := []HeaderDTO{}

	sessions, err := h.sessSvc.Headers(agentID)
	if err != nil {
		// this route reaches only on agent is not exist
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("agent is not exist")
		}

		var BrokenHeadersError *session.BrokenHeadersError
		// if is not a broken headers then is some the real internal problem
		if !errors.As(err, &BrokenHeadersError) {
			return NewInternalError(err)
		}

		// packing broken headers
		for _, e := range BrokenHeadersError.Errors {
			dtos = append(dtos, HeaderDTO{
				ID:    e.SessID,
				Cause: e.Error(),
			})
		}
	}

	for _, header := range sessions {
		dtos = append(dtos, sessHeaderToDTO(header))
	}

	return NewJSONResponse(http.StatusOK, dtos)
}

func (h *sessionHandler) Create(_ http.ResponseWriter, r *http.Request) Response {
	type SessionCreateDTO struct {
		Instructon string `json:"instruction,omitempty"`
	}

	agentID := agent.ID(r.PathValue("agent"))

	requestDTO, err := decode[SessionCreateDTO](r)
	if err != nil {
		return NewInvalidRequest(err)
	}

	sessID, err := h.sessSvc.Create(agentID, requestDTO.Instructon)
	if err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("agent is not exist")
		}
		return NewInternalError(err)
	}

	dto := map[string]any{
		"id": sessID,
	}

	return NewJSONResponse(http.StatusOK, dto)
}

func (h *sessionHandler) Delete(_ http.ResponseWriter, r *http.Request) Response {
	agentID := agent.ID(r.PathValue("agent"))
	sessID := session.ID(r.PathValue("session"))

	if err := h.sessSvc.Delete(agentID, sessID); err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("session is not exist")
		}
		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

func sessHeaderToDTO(header session.Header) HeaderDTO {
	return HeaderDTO{
		ID:           header.ID(),
		InputTokens:  header.InputTokens(),
		OutputTokens: header.OutputTokens(),
		CreatedAt:    header.CreatedAt(),
		UpdatedAt:    header.UpdatedAt(),
		Extras:       header.Extras(),
	}
}
