package common

import (
	"fmt"
	"strings"
	"testing"
)

func TestGetAllProjectUserIntentionsReturnsErrorAfterRetries(t *testing.T) {
	oldLoader := projectIntentionsHGetAll
	defer func() {
		projectIntentionsHGetAll = oldLoader
	}()

	callCount := 0
	projectIntentionsHGetAll = func(hash string) (map[string]string, error) {
		callCount++
		return nil, fmt.Errorf("redis unavailable")
	}

	intentions, err := GetAllProjectUserIntentions()
	if err == nil || !strings.Contains(err.Error(), "redis error after 3 attempts") {
		t.Fatalf("expected retry exhaustion error, got intentions=%v err=%v", intentions, err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 retries, got %d", callCount)
	}
}

func TestGetAllProjectUserIntentionsParsesRunningProjects(t *testing.T) {
	oldLoader := projectIntentionsHGetAll
	defer func() {
		projectIntentionsHGetAll = oldLoader
	}()

	projectIntentionsHGetAll = func(hash string) (map[string]string, error) {
		return map[string]string{
			"project-a": "running",
			"project-b": "stopped",
		}, nil
	}

	intentions, err := GetAllProjectUserIntentions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !intentions["project-a"] {
		t.Fatal("expected project-a to be treated as running")
	}
	if intentions["project-b"] {
		t.Fatal("expected non-running states to map to false")
	}
}
