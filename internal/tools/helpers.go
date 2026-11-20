package tools

import (
	"ard/internal/agent"
	"ard/internal/chat"
	"ard/internal/session"
	"ard/internal/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var _ agent.ToolServer = (*BuildInToolServer)(nil)

type BuildInToolServer struct {
	tools []agent.Tool
}

func NewBuildInToolServer(tools ...agent.Tool) *BuildInToolServer {
	return &BuildInToolServer{
		tools: tools,
	}
}

func (s *BuildInToolServer) Tools() []agent.Tool { return s.tools }

func UnwrapArgs[T any](raw agent.ToolArguments) (T, error) {
	var args T

	if err := json.Unmarshal(raw, &args); err != nil {
		return args, types.NewAgentMistakeError("invalid json aguments")
	}
	return args, nil
}

func UnwrapValidArgs[T types.Validator](ctx context.Context, raw agent.ToolArguments) (T, error) {
	var args T

	if err := json.Unmarshal(raw, &args); err != nil {
		return args, types.NewAgentMistakeError(err.Error())
	}
	if err := args.Validate(ctx); err != nil {
		if validationError, ok := errors.AsType[*types.ValidationError](err); ok {
			var sb strings.Builder
			for k, v := range validationError.Problems() {
				fmt.Fprintf(&sb, "problem with %s - %s", k, v)
			}

			return args, types.NewAgentMistakeError(sb.String())
		}

		return args, err
	}

	return args, nil
}

func MustAgentID(ctx context.Context) agent.ID {
	agentID, _ := chat.AgentIDFromContext(ctx)
	return agentID
}

func MustSessionID(ctx context.Context) session.ID {
	sessionID, _ := chat.SessionIDFromContext(ctx)
	return sessionID
}

func Result[T string | []agent.ContentPart](res T) []agent.ContentPart {
	return agent.NewContent(res)
}
