package common

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

const (
	// Use timer-based sampling: sample once every 3 minutes
	SamplingInterval = 3 * time.Minute // Sample once every 3 minutes
)

// SampleData represents a single sample with its metadata
type SampleData struct {
	Data                interface{} `json:"data"`
	Timestamp           time.Time   `json:"timestamp"`
	ProjectNodeSequence string      `json:"project_node_sequence"`
	// Removed SamplerName as it's not needed for simplified approach
}

// SamplerStats represents statistics about sampling
type SamplerStats struct {
	Name           string           `json:"name"`
	TotalCount     int64            `json:"totalCount"`
	SampledCount   int64            `json:"sampledCount"`
	CurrentSamples int              `json:"currentSamples"`
	MaxSamples     int              `json:"maxSamples"`
	SamplingRate   float64          `json:"samplingRate"`
	ProjectStats   map[string]int64 `json:"projectStats"`
}

// Sampler represents a sampling instance with timer-based sampling
type Sampler struct {
	name              string
	totalCount        uint64
	sampledCount      uint64
	maxSamples        int
	pool              *ants.Pool // Used for asynchronous processing of sampling data
	closed            int32      // Mark whether it's closed
	lastSampleTime    sync.Map   // Cache for last sample time per project sequence
}

// NewSampler creates a new sampler instance
func NewSampler(name string) *Sampler {
	pool, err := ants.NewPool(8, ants.WithPreAlloc(true))
	if err != nil {
		// If creating goroutine pool fails, use default pool
		pool = nil
	}
	
	return &Sampler{
		name:       name,
		maxSamples: 100,
		pool:       pool,
	}
}

// Sample attempts to sample the data based on timer (simplified version)
func (s *Sampler) Sample(data interface{}, projectNodeSequence string) bool {
	// Normalize ProjectNodeSequence to lower-case to avoid case-sensitivity issues downstream
	projectNodeSequence = strings.ToLower(projectNodeSequence)

	// Check if already closed
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	// Check parameter validity
	if data == nil || projectNodeSequence == "" {
		return false
	}

	// Increment total counter using atomic operations
	atomic.AddUint64(&s.totalCount, 1)

	// Check if enough time has passed since last sample for this project sequence
	now := time.Now()
	lastSampleTimeInterface, exists := s.lastSampleTime.Load(projectNodeSequence)
	
	var shouldSample bool
	if !exists {
		// First sample for this project sequence
		shouldSample = true
	} else {
		lastSampleTime := lastSampleTimeInterface.(time.Time)
		// Sample if 3 minutes have passed since last sample
		shouldSample = now.Sub(lastSampleTime) >= SamplingInterval
	}

	if !shouldSample {
		return false
	}

	// Update last sample time
	s.lastSampleTime.Store(projectNodeSequence, now)

	// Increment sampling count
	atomic.AddUint64(&s.sampledCount, 1)

	// Create sample data
	sample := SampleData{
		Data:                data,
		Timestamp:           now,
		ProjectNodeSequence: projectNodeSequence,
	}

	// Store sample asynchronously
	if s.pool != nil && !s.pool.IsClosed() {
		err := s.pool.Submit(func() {
			s.storeSample(sample, projectNodeSequence)
		})
		if err != nil {
			// If submission fails, process synchronously
			s.storeSample(sample, projectNodeSequence)
		}
	} else {
		s.storeSample(sample, projectNodeSequence)
	}

	return true
}

// storeSample stores sample data to Redis only
func (s *Sampler) storeSample(sample SampleData, projectNodeSequence string) {
	redisSampleManager := GetRedisSampleManager()
	if redisSampleManager != nil {
		// Use simplified storage without projectID
		_ = redisSampleManager.StoreSample(s.name, sample)
	}
}

// GetSamples returns all collected samples from Redis
func (s *Sampler) GetSamples() map[string][]SampleData {
	redisSampleManager := GetRedisSampleManager()
	if redisSampleManager == nil {
		return make(map[string][]SampleData)
	}

	samples, err := redisSampleManager.GetSamples(s.name)
	if err != nil {
		return make(map[string][]SampleData)
	}

	return samples
}

// GetStats returns sampling statistics from Redis
func (s *Sampler) GetStats() SamplerStats {
	projectStats := make(map[string]int64)
	totalSamples := 0

	redisSampleManager := GetRedisSampleManager()
	if redisSampleManager != nil {
		stats, err := redisSampleManager.GetStats(s.name)
		if err == nil {
			projectStats = stats
			for _, count := range stats {
				totalSamples += int(count)
			}
		}
	}

	// Calculate actual sampling rate based on timer
	samplingRate := 1.0 / (SamplingInterval.Seconds() / 60) // samples per minute
	if samplingRate > 1.0 {
		samplingRate = 1.0 // Cap at 100%
	}

	return SamplerStats{
		Name:           s.name,
		TotalCount:     int64(atomic.LoadUint64(&s.totalCount)),
		SampledCount:   int64(atomic.LoadUint64(&s.sampledCount)),
		CurrentSamples: totalSamples,
		MaxSamples:     s.maxSamples,
		SamplingRate:   samplingRate,
		ProjectStats:   projectStats,
	}
}

// Reset resets all samples and counters
func (s *Sampler) Reset() {
	atomic.StoreUint64(&s.totalCount, 0)
	atomic.StoreUint64(&s.sampledCount, 0)

	// Clear timer cache
	s.lastSampleTime.Range(func(key, value interface{}) bool {
		s.lastSampleTime.Delete(key)
		return true
	})

	// Clear Redis samples
	redisSampleManager := GetRedisSampleManager()
	if redisSampleManager != nil {
		redisSampleManager.Reset(s.name)
	}
}

// Close closes the sampler and cleans up resources
func (s *Sampler) Close() {
	// Mark as closed
	atomic.StoreInt32(&s.closed, 1)
	
	// Close goroutine pool
	if s.pool != nil {
		s.pool.Release()
		s.pool = nil
	}
}

// Global sampler manager
var (
	samplers = make(map[string]*Sampler)
	mu       sync.RWMutex
)

// GetSampler returns a sampler instance by name
func GetSampler(name string) *Sampler {
	if name == "" {
		return nil
	}

	// Normalize sampler name to lower-case so that all callers map to the same instance
	name = strings.ToLower(name)

	mu.Lock()
	defer mu.Unlock()

	if sampler, exists := samplers[name]; exists {
		return sampler
	}

	sampler := NewSampler(name)
	samplers[name] = sampler
	return sampler
}
