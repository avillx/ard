package openai

import (
	"encoding/json"
	"fmt"
	"sync"

	"ard/internal/agent"
	"ard/internal/model"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Factory struct {
	secrets   SecretsRepo
	clients   map[clientKey]*openai.Client
	clientsMu sync.Mutex
}

func NewFactory(secrets SecretsRepo) *Factory {
	return &Factory{
		secrets: secrets,
		clients: map[clientKey]*openai.Client{},
	}
}

func (f *Factory) APIType() model.APIType {
	return model.APITypeOpenAI
}

func (f *Factory) CreateModel(
	baseURL string,
	keyReference string,
	modelSettings agent.ModelSettings,
) (agent.Model, error) {
	settings, err := f.parseSettings(modelSettings)
	if err != nil {
		return nil, fmt.Errorf("parse model settings: %w", err)
	}

	if settings.Model == nil {
		return nil, fmt.Errorf("model is required")
	}

	client, err := f.getOrCreateClient(baseURL, keyReference)
	if err != nil {
		return nil, fmt.Errorf("get client for provider %s: %w", baseURL, err)
	}

	return NewModel(client, *settings.Model, settings), nil
}

type clientKey struct {
	URL    string
	KeyRef string
}

func (f *Factory) getOrCreateClient(
	BaseURL string,
	KeyReference string,
) (*openai.Client, error) {
	f.clientsMu.Lock()
	defer f.clientsMu.Unlock()

	apiKey, found := f.secrets.Get(KeyReference)
	if !found {
		return nil, fmt.Errorf("api key %s is not found in secrets", KeyReference)
	}

	cacheKey := clientKey{URL: BaseURL, KeyRef: KeyReference}

	if client, ok := f.clients[cacheKey]; ok {
		return client, nil
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(BaseURL),
	)

	f.clients[cacheKey] = &client
	return &client, nil
}

func (f *Factory) parseSettings(raw any) (ModelSettings, error) {
	switch s := raw.(type) {
	case ModelSettings:
		return s, nil
	case agent.ModelSettings:
		data, err := json.Marshal(s)
		if err != nil {
			return ModelSettings{}, err
		}
		var settings ModelSettings
		if err := json.Unmarshal(data, &settings); err != nil {
			return ModelSettings{}, err
		}
		return settings, nil
	default:
		return ModelSettings{}, fmt.Errorf("unexpected model settings type: %T", raw)
	}
}
