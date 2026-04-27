package api

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForFollowerTokenRetriesUntilSuccess(t *testing.T) {
	originalReadFollowerToken := readFollowerToken
	originalSleepBeforeFollowerTokenRetry := sleepBeforeFollowerTokenRetry
	defer func() {
		readFollowerToken = originalReadFollowerToken
		sleepBeforeFollowerTokenRetry = originalSleepBeforeFollowerTokenRetry
	}()

	readCalls := 0
	sleepCalls := 0

	readFollowerToken = func() (string, error) {
		readCalls++
		if readCalls < 3 {
			return "", errors.New("token not ready")
		}
		return "cluster-token", nil
	}
	sleepBeforeFollowerTokenRetry = func(time.Duration) {
		sleepCalls++
	}

	token, err := waitForFollowerToken(5, time.Millisecond)
	if err != nil {
		t.Fatalf("waitForFollowerToken returned unexpected error: %v", err)
	}
	if token != "cluster-token" {
		t.Fatalf("expected token cluster-token, got %q", token)
	}
	if readCalls != 3 {
		t.Fatalf("expected 3 token reads, got %d", readCalls)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 retry sleeps, got %d", sleepCalls)
	}
}

func TestWaitForFollowerTokenReturnsErrorAfterMaxAttempts(t *testing.T) {
	originalReadFollowerToken := readFollowerToken
	originalSleepBeforeFollowerTokenRetry := sleepBeforeFollowerTokenRetry
	defer func() {
		readFollowerToken = originalReadFollowerToken
		sleepBeforeFollowerTokenRetry = originalSleepBeforeFollowerTokenRetry
	}()

	readCalls := 0
	sleepCalls := 0

	readFollowerToken = func() (string, error) {
		readCalls++
		return "", nil
	}
	sleepBeforeFollowerTokenRetry = func(time.Duration) {
		sleepCalls++
	}

	token, err := waitForFollowerToken(3, time.Millisecond)
	if err == nil {
		t.Fatal("expected waitForFollowerToken to fail when token never appears")
	}
	if token != "" {
		t.Fatalf("expected empty token on failure, got %q", token)
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("expected error to mention attempt count, got %v", err)
	}
	if readCalls != 3 {
		t.Fatalf("expected 3 token reads, got %d", readCalls)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 retry sleeps, got %d", sleepCalls)
	}
}
