package profile

import (
	"fmt"
	"slices"
)

type ID string

var _ Profile = (*profile)(nil)

// divide pathch and Merge for avoid saveing external modified profiles
type Profile interface {
	ID() ID
	Description() string
	SystemPrompt() string
	Model() string
	ToolServers() []string
	HasMemory() bool
	Patch(Profile)
	Merge(Profile)
}

type profile struct {
	id           ID
	description  string
	systemPrompt string
	model        string
	toolServers  []string
	hasMemory    bool
}

func NewProfile(
	id ID,
	description string,
	systemPrompt string,
	model string,
	toolServers []string,
	hasMemory bool,
) *profile {
	if toolServers == nil {
		toolServers = []string{}
	}

	return &profile{
		id:           id,
		description:  description,
		systemPrompt: systemPrompt,
		model:        model,
		toolServers:  toolServers,
		hasMemory:    hasMemory,
	}
}

func (p *profile) ID() ID                { return p.id }
func (p *profile) Description() string   { return p.description }
func (p *profile) SystemPrompt() string  { return p.systemPrompt }
func (p *profile) Model() string         { return p.model }
func (p *profile) ToolServers() []string { return p.toolServers }
func (p *profile) HasMemory() bool       { return p.hasMemory }

func (p *profile) Patch(patch Profile) {
	if patch.ID() != "" {
		p.id = patch.ID()
	}

	if patch.Description() != "" {
		p.description = patch.Description()
	}

	if patch.Model() != "" {
		p.model = patch.Model()
	}

	if patch.SystemPrompt() != "" {
		p.systemPrompt = patch.SystemPrompt()
	}

	if patch.ToolServers() != nil && len(patch.ToolServers()) > 0 {
		p.toolServers = patch.ToolServers()
	}

	p.hasMemory = patch.HasMemory()
}

func (p *profile) Merge(secondary Profile) {
	// merge system prompts
	if secondary.SystemPrompt() != "" {
		// Order First is secondary prompt, then profile prompt
		p.systemPrompt = fmt.Sprintf("%s\n\n%s", secondary.SystemPrompt(), p.systemPrompt)
	}

	// merge tool servers
	if secondary.ToolServers() != nil && len(secondary.ToolServers()) > 0 {
		for _, ts := range secondary.ToolServers() {
			if slices.Contains(p.toolServers, ts) {
				continue
			}
			p.toolServers = append(p.toolServers, ts)
		}
	}
}
