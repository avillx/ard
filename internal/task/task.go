package task

import (
	"ard/internal/agent"
	"ard/internal/types"
	"errors"
	"slices"
	"time"
)

const Required = "required"

var (
	ErrIsNotExist   = types.ErrIsNotExist
	ErrAlreadyExist = types.ErrAlreadyExist
	ErrCron         = errors.New("cron is not support this expression")
	ErrNoRecipients = errors.New("must contain at least one recipient")
)

type ValidationError types.ValidationError

type Cron interface {
	NextTime() time.Duration
	Expression() string
}

type Repo interface {
	All() (map[string]Config, error)
	Get(id string) (Config, error)
	Delete(id string) error
	Save(t Config) error
}

type Config struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Recipients  []agent.ID `json:"recipients"`
	Reglament   string     `json:"schedule"`
	Request     string     `json:"request"`
	Active      bool       `json:"active"`
	Oneshot     bool       `json:"oneshot"`
}

func (t Config) Equals(other Config) bool {
	return slices.Equal(t.Recipients, other.Recipients) &&
		t.Name == other.Name &&
		t.Description == other.Description &&
		t.Reglament == other.Reglament &&
		t.Request == other.Request &&
		t.Active == other.Active &&
		t.Oneshot == other.Oneshot
}

type Patch struct {
	Active      *bool       `json:"active,omitempty"`
	Name        *string     `json:"name,omitempty"`
	Description *string     `json:"description,omitempty"`
	Recipients  *[]agent.ID `json:"recipients,omitempty"`
	Reglament   *string     `json:"schedule,omitempty"`
	Request     *string     `json:"request,omitempty"`
	Oneshot     *bool       `json:"oneshot,omitempty"`
}

func applyPatch(cfg Config, patch Patch) Config {
	if patch.Active != nil {
		cfg.Active = *patch.Active
	}

	if patch.Name != nil {
		cfg.Name = *patch.Name
	}

	if patch.Description != nil {
		cfg.Description = *patch.Description
	}

	if patch.Recipients != nil {
		cfg.Recipients = append([]agent.ID{}, (*patch.Recipients)...)
	}

	if patch.Reglament != nil {
		cfg.Reglament = *patch.Reglament
	}

	if patch.Request != nil {
		cfg.Request = *patch.Request
	}

	if patch.Oneshot != nil {
		cfg.Oneshot = *patch.Oneshot
	}

	return cfg
}

func validateConfig(cfg Config, cronFactory func(string) (Cron, error)) error {
	problems := make(map[string]string)
	if cfg.Name == "" {
		problems["name"] = Required
	}
	if cfg.Description == "" {
		problems["description"] = Required
	}
	if len(cfg.Recipients) <= 0 {
		problems["recipients"] = ErrNoRecipients.Error()
	}
	if _, err := cronFactory(cfg.Reglament); err != nil {
		problems["schedule"] = "invalid cron"
	}
	if cfg.Request == "" {
		problems["request"] = Required
	}
	if len(problems) > 0 {
		return types.NewValidationError(problems)
	}

	return nil
}
