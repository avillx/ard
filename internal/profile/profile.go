package profile

type ID string

var _ Profile = (*profile)(nil)

type Profile interface {
	ID() ID
	Description() string
	SystemPrompt() string
	Model() string
	ToolServers() []string
	HasMemory() bool
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

func (a *profile) ID() ID                { return a.id }
func (a *profile) Description() string   { return a.description }
func (a *profile) SystemPrompt() string  { return a.systemPrompt }
func (a *profile) Model() string         { return a.model }
func (a *profile) ToolServers() []string { return a.toolServers }
func (a *profile) HasMemory() bool       { return a.hasMemory }
