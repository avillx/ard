package api

import (
	"ard/internal/profile"
	"ard/internal/types"
	"context"
	"errors"
	"net/http"
)

type AgentDTO struct {
	Model        string   `json:"model,omitempty"`
	Memory       bool     `json:"memory,omitempty"`
	Description  string   `json:"description,omitempty"`
	ToolServers  []string `json:"tool_servers,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
}

func (d AgentDTO) Validate(_ context.Context) error {
	if d.Model == "" {
		return types.NewValidationError(map[string]string{
			"model": "must specified",
		})
	}
	return nil
}

type agentHandler struct {
	repo profile.Repo
}

// GET /agent.
func (h *agentHandler) List(_ http.ResponseWriter, _ *http.Request) Response {
	agents, err := h.repo.All()
	if err != nil {
		return NewInternalError(err)
	}

	dtos := []AgentDTO{}
	for _, agt := range agents {
		dtos = append(dtos, agentToDTO(agt))
	}

	return NewJSONResponse(http.StatusOK, dtos)
}

// POST /agent/{id} DTO.
func (h *agentHandler) Create(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	_, err := h.repo.Get(id)
	if err == nil {
		return NewBadRequest("already exist")
	}
	if !errors.Is(err, types.ErrIsNotExist) {
		return NewInternalError(err)
	}

	return h.saveAgt(id, r)
}

// PUT /agent/{id}.
func (h *agentHandler) Update(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	// To ensure existence
	_, err := h.repo.Get(id)
	if err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("agent is not exist")
		}
		return NewInternalError(err)
	}

	return h.saveAgt(id, r)
}

func (h *agentHandler) saveAgt(agentID profile.ID, r *http.Request) Response {
	agentDTO, err := decode[AgentDTO](r)
	if err != nil {
		return NewInvalidRequest(err)
	}

	if err := h.repo.Save(dtoToAgent(agentID, agentDTO)); err != nil {
		// On Model not exist or ToolServer not exist
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest(err.Error())
		}

		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

// GET /agent/{id} // DTO.
func (h *agentHandler) Read(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))
	agt, err := h.repo.Get(id)
	if err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest("agent is not exist")
		}
		return NewInternalError(err)
	}

	return NewJSONResponse(http.StatusOK, agentToDTO(agt))
}

// DELETE /agent/{id}.
func (h *agentHandler) Delete(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest(err.Error())
		}
		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

func agentToDTO(p profile.Profile) AgentDTO {
	return AgentDTO{
		Model:        p.Model(),
		Memory:       p.HasMemory(),
		Description:  p.Description(),
		ToolServers:  p.ToolServers(),
		SystemPrompt: p.SystemPrompt(),
	}
}

func dtoToAgent(id profile.ID, dto AgentDTO) profile.Profile {
	return profile.NewProfile(
		id,
		dto.Description,
		dto.SystemPrompt,
		dto.Model,
		dto.ToolServers,
		dto.Memory,
	)
}
