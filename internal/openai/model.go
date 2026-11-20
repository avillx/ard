package openai

import (
	"ard/internal/agent"
	"context"
	"slices"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

var _ agent.Model = (*Model)(nil)

const defaultContextLimit = 100_000

type Model struct {
	client     *openai.Client
	modelName  string
	settings   ModelSettings
	settingsMu sync.RWMutex
}

func NewModel(client *openai.Client, modelName string, settings ModelSettings) *Model {
	return &Model{
		client:    client,
		modelName: modelName,
		settings:  settings,
	}
}

func (r *Model) ContextLimit() int64 {
	if r.settings.ContextLimit != nil {
		return *r.settings.ContextLimit
	}
	return defaultContextLimit
}

func (r *Model) Settings() agent.ModelSettings {
	r.settingsMu.RLock()
	defer r.settingsMu.RUnlock()

	return agent.ModelSettings{
		"context_limit":         r.settings.ContextLimit,
		"tool_choice":           r.settings.ToolChoice,
		"reasoning_effort":      r.settings.ReasoningEffort,
		"recall_budget":         r.settings.RecallBudget,
		"temperature":           r.settings.Temperature,
		"top_p":                 r.settings.TopP,
		"frequency_penalty":     r.settings.FrequencyPenalty,
		"presence_penalty":      r.settings.PresencePenalty,
		"max_output_tokens":     r.settings.MaxOutputTokens,
		"max_completion_tokens": r.settings.MaxCompletionTokens,
	}
}

func (r *Model) SupportedModalities() []agent.Modality {
	modalities := r.settings.Modalities
	if modalities == nil {
		return []agent.Modality{agent.TextModality}
	}

	if !slices.Contains(modalities, agent.TextModality) {
		modalities = append(modalities, agent.TextModality)
	}

	return modalities
}

func (r *Model) Complete(
	ctx context.Context,
	tools []agent.Tool,
	internalMsgs []agent.Message,
) (*agent.Completion, error) {
	internalMsgs = excludeUnsupportedModalities(internalMsgs, r.SupportedModalities())
	messages := messagesToOpenAI(internalMsgs)
	agentTools := toolsToOpenAI(tools)
	completionParams := r.buildCompletionParams(messages, agentTools)

	res, err := r.client.Chat.Completions.New(ctx, completionParams)
	if err != nil {
		return nil, err
	}

	return CompletionToReasonResult(res)
}

func (r *Model) buildCompletionParams(
	messages []openai.ChatCompletionMessageParamUnion,
	agentTools []openai.ChatCompletionToolUnionParam,
) openai.ChatCompletionNewParams {
	s := r.settings

	completionParams := openai.ChatCompletionNewParams{
		Messages: messages,
	}

	completionParams.Model = r.modelName

	if agentTools != nil {
		completionParams.Tools = agentTools
	}

	if s.MaxOutputTokens != nil {
		completionParams.MaxTokens = openai.Int(*s.MaxOutputTokens)
	}

	if s.MaxCompletionTokens != nil {
		completionParams.MaxCompletionTokens = openai.Int(*s.MaxCompletionTokens)
	}

	if s.ReasoningEffort != nil {
		completionParams.ReasoningEffort = shared.ReasoningEffort(*s.ReasoningEffort)
	}

	if s.ToolChoice != nil {
		completionParams.ToolChoice = ToolChoice(*s.ToolChoice)
	}

	if s.TopP != nil {
		completionParams.TopP = openai.Float(float64(*s.TopP))
	}

	if s.Temperature != nil {
		completionParams.Temperature = openai.Float(float64(*s.Temperature))
	}

	if s.FrequencyPenalty != nil {
		completionParams.FrequencyPenalty = openai.Float(float64(*s.FrequencyPenalty))
	}

	if s.PresencePenalty != nil {
		completionParams.PresencePenalty = openai.Float(float64(*s.PresencePenalty))
	}

	if s.Extras != nil {
		completionParams.SetExtraFields(s.Extras)
	}

	return completionParams
}

func excludeUnsupportedModalities(msgs []agent.Message, mdls []agent.Modality) []agent.Message {
	if !slices.Contains(mdls, agent.ImageModality) {
		msgs = excludeImageModality(msgs)
	}

	return msgs
}

func excludeImageModality(msgs []agent.Message) []agent.Message {
	const unsupportedLabel = " image modality unsupported by model"
	for _, msg := range msgs {
		contentParts := msg.Content()
		safeContent := make([]agent.ContentPart, 0, len(contentParts))
		for _, contentPart := range contentParts {
			if contentPart.ImageURL != "" {
				contentPart.Text += unsupportedLabel
				contentPart.ImageURL = ""
			}
			safeContent = append(safeContent, contentPart)
		}
		msg.SetContent(safeContent)
	}
	return msgs
}
