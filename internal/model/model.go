package model

import (
	"ard/internal/agent"
	"ard/internal/types"
	"strings"
	"sync"
)

var _ agent.ModelRegistry = (*Service)(nil)

type Service struct {
	factories map[APIType]Factory
	models    map[ID]agent.Model
	mu        sync.RWMutex
}

func NewService(factories ...TypedFactory) *Service {
	svc := &Service{
		factories: make(map[APIType]Factory),
		models:    make(map[ID]agent.Model),
	}

	for _, f := range factories {
		svc.factories[f.APIType()] = f
	}

	return svc
}

func (m *Service) Get(modelName string) (agent.Model, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for k, v := range m.models {
		if strings.HasSuffix(string(k), modelName) {
			return v, nil
		}
	}

	return nil, types.ErrIsNotExist
}

func (m *Service) delete(modelName ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.models, modelName)
}

func (m *Service) add(
	apiType APIType,
	baseURL string,
	keyReference string,
	modelID ID,
	modelSettings agent.ModelSettings,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, ok := m.factories[apiType]
	if !ok {
		return ErrUnsupportedAPI
	}

	model, err := f.CreateModel(baseURL, keyReference, modelSettings)
	if err != nil {
		return err
	}

	m.models[modelID] = model

	return nil
}
