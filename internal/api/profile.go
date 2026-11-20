package api

import (
	"ard/internal/profile"
	"ard/internal/types"
	"context"
	"errors"
	"net/http"
)

type ProfileDTO struct {
	Model        string   `json:"model,omitempty"`
	Memory       bool     `json:"memory,omitempty"`
	Description  string   `json:"description,omitempty"`
	ToolServers  []string `json:"tool_servers,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
}

func (d ProfileDTO) Validate(_ context.Context) error {
	if d.Model == "" {
		return types.NewValidationError(map[string]string{
			"model": "must specified",
		})
	}
	return nil
}

type profileHandler struct {
	svc *profile.Service
}

// GET /profile.
func (h *profileHandler) List(_ http.ResponseWriter, _ *http.Request) Response {
	profiles, err := h.svc.All()
	if err != nil {
		return NewInternalError(err)
	}

	dto := map[profile.ID]ProfileDTO{}
	for _, p := range profiles {
		dto[p.ID()] = profileToDTO(p)
	}

	return NewJSONResponse(http.StatusOK, dto)
}

// POST /profile/{id} DTO.
func (h *profileHandler) Create(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	ok, err := h.svc.IsExist(id)
	if err != nil {
		return NewInternalError(err)
	}

	if ok {
		return NewBadRequest("already exist")
	}

	return h.saveAgt(id, r)
}

// PUT /profile/{id}.
func (h *profileHandler) Update(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	// To ensure existence
	//
	exist, err := h.svc.IsExist(id)
	if err != nil {
		return NewInternalError(err)
	}

	if !exist {
		return NewBadRequest("agent is not exist")
	}

	return h.saveAgt(id, r)
}

func (h *profileHandler) saveAgt(agentID profile.ID, r *http.Request) Response {
	profileDTO, err := decode[ProfileDTO](r)
	if err != nil {
		return NewInvalidRequest(err)
	}

	if err := h.svc.Save(dtoToProfile(agentID, profileDTO)); err != nil {
		// On Model not exist or ToolServer not exist
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest(err.Error())
		}

		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

// GET /profile/{id} // DTO.
func (h *profileHandler) Read(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	exist, err := h.svc.IsExist(id)
	if err != nil {
		return NewInternalError(err)
	}

	if !exist {
		return NewBadRequest("agent is not exist")
	}

	p, err := h.svc.Get(id)
	if err != nil {
		return NewInternalError(err)
	}

	return NewJSONResponse(http.StatusOK, profileToDTO(p))
}

// DELETE /profile/{id}.
func (h *profileHandler) Delete(_ http.ResponseWriter, r *http.Request) Response {
	id := profile.ID(r.PathValue("id"))

	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, types.ErrIsNotExist) {
			return NewBadRequest(err.Error())
		}
		return NewInternalError(err)
	}

	return NewResponse(http.StatusOK)
}

func profileToDTO(p profile.Profile) ProfileDTO {
	return ProfileDTO{
		Model:        p.Model(),
		Memory:       p.HasMemory(),
		Description:  p.Description(),
		ToolServers:  p.ToolServers(),
		SystemPrompt: p.SystemPrompt(),
	}
}

func dtoToProfile(id profile.ID, dto ProfileDTO) profile.Profile {
	return profile.NewProfile(
		id,
		dto.Description,
		dto.SystemPrompt,
		dto.Model,
		dto.ToolServers,
		dto.Memory,
	)
}
