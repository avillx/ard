package chat

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/prompt"
	"ard/internal/session"
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type SystemMessageParticipant interface {
	Part(ctx context.Context) (string, error)
}

var _ SystemMessageBuilder = (*ContextAssembler)(nil)

type ContextAssembler struct {
	skillRepo  SkillsRepo
	memoryRepo ConsolidatorRepo
	logger     *slog.Logger
}

func NewContextAssembler(
	skillRepo SkillsRepo,
	memoryRepo ConsolidatorRepo,
	logger *slog.Logger,
) *ContextAssembler {
	return &ContextAssembler{
		skillRepo:  skillRepo,
		memoryRepo: memoryRepo,
		logger:     logger.WithGroup("context_assembler"),
	}
}

func (a *ContextAssembler) BuildSystemMessage(
	ctx context.Context,
	p profile.Profile,
	toolServers []agent.ToolServer,
	sess session.Session,
) (*agent.SystemMessage, error) {
	parts := []SystemMessageParticipant{
		&ProflilePart{p: p},
		&MemoryPart{p: p, repo: a.memoryRepo},
		&SkillPart{repo: a.skillRepo, logger: a.logger},
		&ToolAwarePart{toolServers: toolServers},
		&SessionPart{sess: sess},
	}

	text, err := AssembleParts(ctx, parts)
	if err != nil {
		return nil, err
	}

	return agent.NewSystemMessage(text), nil
}

func AssembleParts(ctx context.Context, parts []SystemMessageParticipant) (string, error) {
	var systemMessage strings.Builder
	for _, p := range parts {
		text, err := p.Part(ctx)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&systemMessage, "%s\n\n", text)
	}
	return systemMessage.String(), nil
}

// agent system Prompt.
var _ SystemMessageParticipant = (*ProflilePart)(nil)

type ProflilePart struct {
	p profile.Profile
}

func (p *ProflilePart) Part(_ context.Context) (string, error) {
	return p.p.SystemPrompt(), nil
}

// memory index.
var _ SystemMessageParticipant = (*MemoryPart)(nil)

type ConsolidatorRepo interface {
	// expected map[/path/to/memory_file]memory_description
	MemoryIndex() (map[string]string, error)
}

type MemoryPart struct {
	p    profile.Profile
	repo ConsolidatorRepo
}

func (i *MemoryPart) Part(_ context.Context) (string, error) {
	if !i.p.HasMemory() {
		return "", nil
	}

	// load persistent memory prompt
	idx, err := i.repo.MemoryIndex()
	if err != nil {
		return "", fmt.Errorf("memory index is not reached: %w", err)
	}

	// has no memories, no need to guide it
	if len(idx) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for k, v := range idx {
		fmt.Fprintf(&sb, "- (%s) %s\n", k, v)
	}

	return prompt.PersistentMemory(sb.String()), nil
}

// skill index.
var _ SystemMessageParticipant = (*SkillPart)(nil)

type SkillsRepo interface {
	// expected: map[/path/to/skill]skill_description
	Skills() (map[string]string, error)
}

type SkillPart struct {
	repo   SkillsRepo
	logger *slog.Logger
}

func (p *SkillPart) Part(ctx context.Context) (string, error) {
	skills, err := p.repo.Skills()
	if err != nil {
		p.logger.ErrorContext(ctx, "skill load", "error", err)
	}

	// has no skills no need to guide it
	if len(skills) == 0 {
		return "", nil
	}

	// build index
	var sb strings.Builder
	for p, desc := range skills {
		fmt.Fprintf(&sb, "- (%s) %s\n", p, desc)
	}

	return prompt.SkillGuidance(sb.String()), nil
}

// tool aware behaviour.
var _ SystemMessageParticipant = (*ToolAwarePart)(nil)

type ToolInstructer interface {
	Instruction() string
}

// type PerAgentToolInstructer interface {
// 	AgentInstruction(agent.Agent) string
// }

type ToolAwarePart struct {
	toolServers []agent.ToolServer
}

func (i *ToolAwarePart) Part(_ context.Context) (string, error) {
	// if has no tools no need to guide it
	if len(i.toolServers) == 0 {
		return "", nil
	}

	instructions := []string{}

	if len(i.toolServers) > 0 {
		instructions = append(instructions, prompt.ToolUsageGuide())
	}

	for _, t := range i.toolServers {
		if instructedTool, ok := t.(ToolInstructer); ok {
			instructions = append(instructions, instructedTool.Instruction())
		}
		// if agentInstructedTool, ok := t.(PerAgentToolInstructer); ok {
		// 	instructions = append(instructions, agentInstructedTool.AgentInstruction(i.agt))
		// }
	}

	return strings.Join(instructions, "\n\n"), nil
}

// session additional context.
var _ SystemMessageParticipant = (*SessionPart)(nil)

type SessionPart struct {
	sess session.Session
}

func (i *SessionPart) Part(_ context.Context) (string, error) {
	extras := i.sess.Extras()
	if len(extras) == 0 {
		return "", nil
	}

	instruction, ok := extras["instruction"]
	if !ok {
		return "", nil
	}

	stringInstruction, ok := instruction.(string)
	if !ok {
		return "", fmt.Errorf("session instruction: bad type: want string, have: %T", instruction)
	}

	return stringInstruction, nil
}
