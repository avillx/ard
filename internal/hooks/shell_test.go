package hooks_test

import (
	"ard/internal/hooks"
	"testing"
)

func TestNewShellHook(t *testing.T) {
	if _, err := hooks.NewShellHook(); err != nil {
		t.Fatalf("NewShellHook() error = %v", err)
	}
}
