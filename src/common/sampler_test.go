package common

import (
	"testing"
	"time"
)

func TestSampler(t *testing.T) {
	// Create sampler
	sampler := NewSampler("test")
	if sampler == nil {
		t.Fatal("Failed to create sampler")
	}
	defer sampler.Close()

	// Test basic sampling functionality
	testData := map[string]interface{}{
		"message": "test message",
		"id":      123,
	}

	// Since sampling rate is 1/64, we need to send enough data to have sampling results
	sampleCount := 0
	for i := 0; i < 10000; i++ {
		if sampler.Sample(testData, "test", "test-project") {
			sampleCount++
		}
	}

	// Check statistics
	stats := sampler.GetStats()
	if stats.TotalCount != 10000 {
		t.Errorf("Expected total count 10000, got %d", stats.TotalCount)
	}

	if stats.SampledCount == 0 {
		t.Error("Expected some samples, got 0")
	}

	// Sampling rate should be approximately 1/64 (allow some error)
	actualRate := float64(stats.SampledCount) / float64(stats.TotalCount)
	expectedRate := 0.001
	if actualRate < expectedRate*0.5 || actualRate > expectedRate*2 {
		t.Errorf("Expected sampling rate around %f, got %f", expectedRate, actualRate)
	}

	// Wait for asynchronous processing to complete
	time.Sleep(100 * time.Millisecond)

	// Check sampling data
	samples := sampler.GetSamplesByProject("test-project")
	if len(samples) == 0 {
		t.Error("Expected some sample data, got none")
	}

	// Verify sampling data structure
	if len(samples) > 0 {
		sample := samples[0]
		if sample.Source != "test" {
			t.Errorf("Expected source 'test', got '%s'", sample.Source)
		}
		if sample.ProjectNodeSequence != "test-project" {
			t.Errorf("Expected project node sequence 'test-project', got '%s'", sample.ProjectNodeSequence)
		}
	}
}

func TestSamplerEdgeCases(t *testing.T) {
	// Test empty parameters
	sampler := NewSampler("test")
	defer sampler.Close()

	// Empty data should return false
	if sampler.Sample(nil, "test", "test-project") {
		t.Error("Expected false for nil data")
	}

	// Empty source should return false
	if sampler.Sample("data", "", "test-project") {
		t.Error("Expected false for empty source")
	}

	// Empty projectNodeSequence should return false
	if sampler.Sample("data", "test", "") {
		t.Error("Expected false for empty project node sequence")
	}
}

func TestSamplerClose(t *testing.T) {
	sampler := NewSampler("test")

	// Close sampler
	sampler.Close()

	// Should reject new sampling after closing
	if sampler.Sample("data", "test", "test-project") {
		t.Error("Expected false after sampler is closed")
	}
}

func TestGetSampler(t *testing.T) {
	// Test empty name
	sampler := GetSampler("")
	if sampler != nil {
		t.Error("Expected nil for empty name")
	}

	// Test normal retrieval
	sampler1 := GetSampler("test1")
	if sampler1 == nil {
		t.Error("Expected non-nil sampler")
	}

	// Getting again should return the same instance
	sampler2 := GetSampler("test1")
	if sampler1 != sampler2 {
		t.Error("Expected same sampler instance")
	}

	// Cleanup
	sampler1.Close()
}
