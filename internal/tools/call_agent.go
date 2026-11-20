package tools

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/subagent"
	"ard/internal/types"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type CallAgentToolServer struct {
	*BuildInToolServer

	logger       *slog.Logger
	profilesRepo profile.Repo
}

func NewCallAgentToolServer(s *subagent.Service, profilesRepo profile.Repo, logger *slog.Logger) *CallAgentToolServer {
	return &CallAgentToolServer{
		profilesRepo: profilesRepo,
		logger:       logger.WithGroup("call_agent_tool"),
		BuildInToolServer: NewBuildInToolServer(
			&CallAgentTool{
				subagentSvc: s,
			},
		),
	}
}

func (t *CallAgentToolServer) Instruction() string {
	const instruction = `## Call Agents:
You can call sub agent for delegateing task. To agent with diffirent capabilities.
Sub agents is your side instance with clean context,
Do it when operation too complex (5+ toolcalls).
Request should be exhaustive: e.g. task, details, context and expected result.
Called sub agent is stateless. If sub agent clarify something
then sub agent need full request with clarificaton again.
`

	profiles, err := t.profilesRepo.All()
	if err != nil {
		t.logger.Error("call agent tool", "error", "can't gather agents")
		return ""
	}

	var sb strings.Builder

	sb.WriteString(instruction + "\n")
	sb.WriteString("Profiles:\n")
	for _, a := range profiles {
		fmt.Fprintf(&sb, "%s - %s\n", string(a.ID()), a.Description())
		fmt.Fprintf(&sb, "tools: %s\n\n", strings.Join(a.ToolServers(), ", "))
	}

	return sb.String()
}

type CallAgentTool struct {
	subagentSvc *subagent.Service
}

// a sub agent works autonomously, so it gets a generous deadline.
const subAgentCallTimeout = 10 * time.Minute

func (t *CallAgentTool) Name() agent.ToolName {
	return "call_agent"
}

func (t *CallAgentTool) Description() string {
	return "Ask question or delegate a task to sub agent"
}

func (t *CallAgentTool) TimeOut() time.Duration {
	return subAgentCallTimeout
}

func (t *CallAgentTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Target agent name",
			},
			"request": map[string]any{
				"type":        "string",
				"description": "ask or question for agent",
			},
		},
		"required": []string{"name", "request"},
	}
}

func (t *CallAgentTool) Call(ctx context.Context, rawArgs agent.ToolArguments) ([]agent.ContentPart, error) {
	args, err := UnwrapArgs[struct {
		Name    profile.ID `json:"name"`
		Request string     `json:"request"`
	}](rawArgs)
	if err != nil {
		return nil, err
	}

	res, err := t.subagentSvc.Call(
		ctx,
		args.Name,
		args.Request,
	)
	if err != nil {
		if errors.Is(err, subagent.ErrCallStackOverflow) {
			msg := "You already called as sub agent, solove problem by youself"
			return nil, types.NewAgentMistakeError(msg)
		}

		if errors.Is(err, types.ErrIsNotExist) {
			return nil, types.NewAgentMistakeError(err.Error())
		}

		return nil, types.NewAgentMistakeError("problem to call agent %s")
	}

	msg := fmt.Sprintf("# Agent %s respones:\n%s", args.Name, res)
	return Result(msg), nil
}
