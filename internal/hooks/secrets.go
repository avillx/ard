package hooks

import (
	"context"

	"ard/internal/agent"
	"ard/internal/runtime"
)

type Replcaer interface {
	Replace(text string) string
}

var _ runtime.CompletionHook = (*NeverTypeSecretsHook)(nil)

type NeverTypeSecretsHook struct {
	r Replcaer
}

func (h *NeverTypeSecretsHook) Apply(
	_ context.Context,
	c *agent.Completion,
) (*agent.Completion, error) {
	c.Content = h.r.Replace(c.Content)
	return c, nil
}

var _ runtime.ToolCallHook = (*NeverToolCallSecretsHook)(nil)

type NeverToolCallSecretsHook struct {
	r Replcaer
}

func (h *NeverToolCallSecretsHook) Apply(
	_ context.Context,
	c *agent.ToolCall,
) (*agent.ToolCall, error) {
	stringArgs := string(c.Arguments)
	safeString := h.r.Replace(stringArgs)
	c.Arguments = []byte(safeString)

	return c, nil
}

var _ runtime.ToolResultHook = (*NeverReadSecretsHook)(nil)

type NeverReadSecretsHook struct {
	r Replcaer
}

func (h *NeverReadSecretsHook) Apply(
	_ context.Context,
	c *runtime.AfterToolCall,
) (*runtime.AfterToolCall, error) {
	safeCps := []agent.ContentPart{}
	for _, cp := range c.Result {
		safe := h.r.Replace(cp.Text)
		safeCps = append(safeCps, agent.ContentPart{Text: safe, ImageURL: cp.ImageURL})
	}

	c.Result = safeCps

	return c, nil
}
