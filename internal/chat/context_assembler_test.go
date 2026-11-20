package chat_test

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/profile"
	"ard/internal/session"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// mockConsolidatorRepo implements chat.ConsolidatorRepo.
type mockConsolidatorRepo struct {
	idx map[string]string
	err error
}

func (m *mockConsolidatorRepo) MemoryIndex() (map[string]string, error) {
	return m.idx, m.err
}

// mockSkillsRepo implements chat.SkillsRepo.
type mockSkillsRepo struct {
	skills map[string]string
}

func (m *mockSkillsRepo) Skills() (map[string]string, error) {
	return m.skills, nil
}

// plain tool server without any instruction interfaces.
type plainToolServer struct{}

func (p *plainToolServer) Tools() []agent.Tool { return nil }

// instructedToolServer implements chat.ToolInstructer.
type instructedToolServer struct {
	instruction string
}

func (s *instructedToolServer) Tools() []agent.Tool { return nil }
func (s *instructedToolServer) Instruction() string { return s.instruction }

func newAssemblerProfile(prompt string, hasMemory bool) profile.Profile {
	return profile.NewProfile("agent-1", "test profile", prompt, "model-1", nil, hasMemory)
}

func newAssemblerSession(extras map[string]any) session.Session {
	if extras == nil {
		extras = map[string]any{}
	}
	return session.NewRestoredSession(
		session.NewHeader("sess-1", 0, 0, time.Now(), time.Now(), extras),
		nil,
	)
}

func buildSystemMessage(
	t *testing.T,
	memoryRepo chat.ConsolidatorRepo,
	skillRepo chat.SkillsRepo,
	p profile.Profile,
	toolServers []agent.ToolServer,
	sess session.Session,
) (string, error) {
	t.Helper()

	assembler := chat.NewContextAssembler(skillRepo, memoryRepo, slog.New(slog.DiscardHandler))
	msg, err := assembler.BuildSystemMessage(context.Background(), p, toolServers, sess)
	if err != nil {
		return "", err
	}

	content := msg.Content()
	if len(content) != 1 {
		t.Fatalf("expected single content part, got %d", len(content))
	}
	return content[0].Text, nil
}

func requireContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected %q to contain %q", text, want)
	}
}

func requireNotContains(t *testing.T, text, want string) {
	t.Helper()
	if strings.Contains(text, want) {
		t.Fatalf("expected %q to not contain %q", text, want)
	}
}

func requireInOrder(t *testing.T, text string, markers ...string) {
	t.Helper()

	last := -1
	for _, marker := range markers {
		idx := strings.Index(text, marker)
		if idx < 0 {
			t.Fatalf("expected %q to contain %q", text, marker)
		}
		if idx <= last {
			t.Fatalf("expected %q to appear after the previously found marker in %q", marker, text)
		}
		last = idx
	}
}

func TestBuildSystemMessage_HappyPath(t *testing.T) {
	const profilePrompt = "you are a test profile"
	const memoryOnePath = "/memory/tasks.md"
	const memoryTwoPath = "/memory/notes.md"
	const skillOnePath = "/skills/py/SKILL.md"
	const skillTwoPath = "/skills/sh/SKILL.md"
	const toolInstruction = "use shell carefully"
	const sessionInstruction = "answer in russian"

	memoryRepo := &mockConsolidatorRepo{idx: map[string]string{
		memoryOnePath: "task memory",
		memoryTwoPath: "notes memory",
	}}
	skillRepo := &mockSkillsRepo{skills: map[string]string{
		skillOnePath: "python skill",
		skillTwoPath: "shell skill",
	}}

	toolServers := []agent.ToolServer{
		&plainToolServer{},
		&instructedToolServer{instruction: toolInstruction},
	}

	text, err := buildSystemMessage(
		t,
		memoryRepo,
		skillRepo,
		newAssemblerProfile(profilePrompt, true),
		toolServers,
		newAssemblerSession(map[string]any{"instruction": sessionInstruction}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// every part contributes its content
	for _, want := range []string{
		profilePrompt,
		memoryOnePath,
		memoryTwoPath,
		skillOnePath,
		skillTwoPath,
		toolInstruction,
		sessionInstruction,
	} {
		requireContains(t, text, want)
	}

	// parts keep the assembler order
	requireInOrder(
		t,
		text,
		profilePrompt,
		memoryOnePath,
		skillOnePath,
		toolInstruction,
		sessionInstruction,
	)
}

func TestProfilePart_IncludesCustomSystemPrompt(t *testing.T) {
	const prompt = "custom system prompt"

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile(prompt, false), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, prompt)
}

func TestProfilePart_FallsBackToDefaultPrompt(t *testing.T) {
	const prompt = "custom system prompt"

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if text == "" {
		t.Fatal("expected non-empty default prompt")
	}
	requireNotContains(t, text, prompt)
}

func TestMemoryPart_IncludesMemoryIndex(t *testing.T) {
	const path = "/memory/notes.md"
	const desc = "notes about the project"

	repo := &mockConsolidatorRepo{idx: map[string]string{path: desc}}

	text, err := buildSystemMessage(t, repo, &mockSkillsRepo{},
		newAssemblerProfile("", true), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, "- ("+path+") "+desc)
}

func TestMemoryPart_SkipsEmptyIndex(t *testing.T) {
	const path = "/memory/notes.md"

	repo := &mockConsolidatorRepo{idx: map[string]string{}}

	text, err := buildSystemMessage(t, repo, &mockSkillsRepo{},
		newAssemblerProfile("", true), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireNotContains(t, text, path)
}

func TestMemoryPart_ErrorWhenIndexNotReached(t *testing.T) {
	repo := &mockConsolidatorRepo{err: errors.New("memory repo is down")}

	_, err := buildSystemMessage(t, repo, &mockSkillsRepo{},
		newAssemblerProfile("", true), nil,
		newAssemblerSession(nil))
	if err == nil {
		t.Fatal("expected error when memory index is not reached")
	}
}

func TestSkillPart_IncludesSkillIndex(t *testing.T) {
	const path = "/skills/py/SKILL.md"
	const desc = "python coding skill"

	repo := &mockSkillsRepo{skills: map[string]string{path: desc}}

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, repo,
		newAssemblerProfile("", false), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, "- ("+path+") "+desc)
}

func TestSkillPart_SkipsEmptyIndex(t *testing.T) {
	const path = "/skills/py/SKILL.md"

	repo := &mockSkillsRepo{skills: map[string]string{}}

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, repo,
		newAssemblerProfile("", false), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireNotContains(t, text, path)
}

func TestToolAwarePart_IncludesToolInstructions(t *testing.T) {
	const toolInstruction = "shell tool instruction"

	toolServers := []agent.ToolServer{
		&instructedToolServer{instruction: toolInstruction},
	}

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), toolServers,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, toolInstruction)
}

func TestToolAwarePart_IgnoresServersWithoutInstructions(t *testing.T) {
	const toolInstruction = "shell tool instruction"

	toolServers := []agent.ToolServer{
		&plainToolServer{},
		&instructedToolServer{instruction: toolInstruction},
	}

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), toolServers,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, toolInstruction)
}

func TestToolAwarePart_SkipsWhenNoTools(t *testing.T) {
	const toolInstruction = "shell tool instruction"

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), nil,
		newAssemblerSession(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireNotContains(t, text, toolInstruction)
}

func TestSessionPart_IncludesInstruction(t *testing.T) {
	const instruction = "always answer in english"

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), nil,
		newAssemblerSession(map[string]any{"instruction": instruction}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireContains(t, text, instruction)
}

func TestSessionPart_SkipsWithoutInstruction(t *testing.T) {
	const instruction = "always answer in english"

	text, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), nil,
		newAssemblerSession(map[string]any{"other": "no instruction here"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requireNotContains(t, text, instruction)
}

func TestSessionPart_ErrorOnBadInstructionType(t *testing.T) {
	_, err := buildSystemMessage(t, &mockConsolidatorRepo{}, &mockSkillsRepo{},
		newAssemblerProfile("", false), nil,
		newAssemblerSession(map[string]any{"instruction": 42}))
	if err == nil {
		t.Fatal("expected error when session instruction is not a string")
	}
}
