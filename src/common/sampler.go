package common

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

const (
	// Use bitwise operation to optimize sampling rate calculation
	SamplingMask = 63 // 2^6 - 1, corresponding to 1/64 sampling rate (sample 1 out of every 64 messages)
)

// SampleData represents a single sample with its metadata
type SampleData struct {
	Data                interface{} `json:"data"`
	Timestamp           time.Time   `json:"timestamp"`
	ProjectNodeSequence string      `json:"project_node_sequence"`
}

// ProjectSamples uses lock-free ring buffer
type ProjectSamples struct {
	samples     [100]SampleData // Fixed-size ring buffer
	writeIdx    uint32          // Write position
	sampleCount uint32          // Current sample count
	mu          sync.RWMutex    // Read-write lock to protect data consistency
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

// Sampler represents a sampling instance
type Sampler struct {
	name         string
	samples      sync.Map // key: projectNodeSequence, value: *ProjectSamples
	totalCount   uint64
	sampledCount uint64
	maxSamples   int
	pool         *ants.Pool // Used for asynchronous processing of sampling data
	closed       int32      // Mark whether it's closed
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

// getOrCreateProjectSamples gets or creates project sampler
func (s *Sampler) getOrCreateProjectSamples(projectNodeSequence string) *ProjectSamples {
	value, _ := s.samples.LoadOrStore(projectNodeSequence, &ProjectSamples{})
	return value.(*ProjectSamples)
}

// Sample attempts to sample the data based on sampling rate
func (s *Sampler) Sample(data interface{}, projectNodeSequence string) bool {
	// Check if already closed
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	// Check parameter validity
	if data == nil || projectNodeSequence == "" {
		return false
	}

	// Increment counter using atomic operations
	total := atomic.AddUint64(&s.totalCount, 1)

	// Check if it's the first data or the first data for this ProjectNodeSequence
	shouldSampleFirst := false

	// Check if it's the first data for this ProjectNodeSequence
	ps := s.getOrCreateProjectSamples(projectNodeSequence)
	ps.mu.RLock()
	isEmpty := ps.sampleCount == 0
	ps.mu.RUnlock()

	if isEmpty {
		// If this ProjectNodeSequence has no samples yet, force collection of the first one
		shouldSampleFirst = true
	}

	// Sampling decision: force collection of first data, or collect according to sampling rate
	shouldSample := shouldSampleFirst || (total&SamplingMask == 0)

	if !shouldSample {
		return false
	}

	// Increment sampling count
	atomic.AddUint64(&s.sampledCount, 1)

	// Create sample data
	sample := SampleData{
		Data:                data,
		Timestamp:           time.Now(),
		ProjectNodeSequence: projectNodeSequence,
	}

	// If there's a goroutine pool, process asynchronously; otherwise process synchronously
	if s.pool != nil {
		// Check if goroutine pool is closed
		if s.pool.IsClosed() {
			s.storeSample(sample, projectNodeSequence)
		} else {
			err := s.pool.Submit(func() {
				s.storeSample(sample, projectNodeSequence)
			})
			if err != nil {
				// If submission fails, process synchronously
				s.storeSample(sample, projectNodeSequence)
			}
		}
	} else {
		s.storeSample(sample, projectNodeSequence)
	}

	return true
}

// storeSample stores sample data to ring buffer
func (s *Sampler) storeSample(sample SampleData, projectNodeSequence string) {
	// Get project sampler
	ps := s.getOrCreateProjectSamples(projectNodeSequence)

	// Use write lock to protect write operations
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Write sample data to ring buffer
	writeIdx := ps.writeIdx % 100
	ps.samples[writeIdx] = sample
	ps.writeIdx++

	// Update sample count
	if ps.sampleCount < 100 {
		ps.sampleCount++
	}
}

// GetSamples returns all collected samples
func (s *Sampler) GetSamples() map[string][]SampleData {
	result := make(map[string][]SampleData)

	s.samples.Range(func(key, value interface{}) bool {
		projectNodeSequence := key.(string)
		ps := value.(*ProjectSamples)

		// Use read lock to protect read operations
		ps.mu.RLock()
		count := ps.sampleCount
		writeIdx := ps.writeIdx

		if count == 0 {
			ps.mu.RUnlock()
			return true
		}

		// Copy sample data
		samples := make([]SampleData, count)

		// Start copying from oldest data (if buffer is full)
		if count == 100 {
			// Buffer is full, start from oldest data
			startIdx := writeIdx % 100
			for i := uint32(0); i < count; i++ {
				idx := (startIdx + i) % 100
				samples[i] = ps.samples[idx]
			}
		} else {
			// Buffer is not full, start from index 0
			for i := uint32(0); i < count; i++ {
				samples[i] = ps.samples[i]
			}
		}

		ps.mu.RUnlock()
		result[projectNodeSequence] = samples
		return true
	})

	return result
}

// GetSamplesByProject returns samples for a specific project
func (s *Sampler) GetSamplesByProject(projectNodeSequence string) []SampleData {
	if value, ok := s.samples.Load(projectNodeSequence); ok {
		ps := value.(*ProjectSamples)

		// Use read lock to protect read operations
		ps.mu.RLock()
		defer ps.mu.RUnlock()

		count := ps.sampleCount
		writeIdx := ps.writeIdx

		if count == 0 {
			return nil
		}

		// Copy sample data
		samples := make([]SampleData, count)

		// Start copying from oldest data (if buffer is full)
		if count == 100 {
			// Buffer is full, start from oldest data
			startIdx := writeIdx % 100
			for i := uint32(0); i < count; i++ {
				idx := (startIdx + i) % 100
				samples[i] = ps.samples[idx]
			}
		} else {
			// Buffer is not full, start from index 0
			for i := uint32(0); i < count; i++ {
				samples[i] = ps.samples[i]
			}
		}

		return samples
	}
	return nil
}

// GetStats returns sampling statistics
func (s *Sampler) GetStats() SamplerStats {
	projectStats := make(map[string]int64)
	totalSamples := 0

	s.samples.Range(func(key, value interface{}) bool {
		projectNodeSequence := key.(string)
		ps := value.(*ProjectSamples)

		ps.mu.RLock()
		count := ps.sampleCount
		ps.mu.RUnlock()

		projectStats[projectNodeSequence] = int64(count)
		totalSamples += int(count)
		return true
	})

	return SamplerStats{
		Name:           s.name,
		TotalCount:     int64(atomic.LoadUint64(&s.totalCount)),
		SampledCount:   int64(atomic.LoadUint64(&s.sampledCount)),
		CurrentSamples: totalSamples,
		MaxSamples:     s.maxSamples,
		SamplingRate:   0.015625, // 1/64 = 0.015625
		ProjectStats:   projectStats,
	}
}

// Reset resets all samples and counters
func (s *Sampler) Reset() {
	atomic.StoreUint64(&s.totalCount, 0)
	atomic.StoreUint64(&s.sampledCount, 0)
	s.samples = sync.Map{}
}

// ResetProject resets samples for a specific project
func (s *Sampler) ResetProject(projectNodeSequence string) {
	s.samples.Delete(projectNodeSequence)
}

// Close releases resources
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

	mu.Lock()
	defer mu.Unlock()

	if sampler, exists := samplers[name]; exists {
		return sampler
	}

	sampler := NewSampler(name)
	samplers[name] = sampler
	return sampler
}

// GetAllSamplers returns all sampler instances
func GetAllSamplers() map[string]*Sampler {
	mu.RLock()
	defer mu.RUnlock()

	result := make(map[string]*Sampler)
	for k, v := range samplers {
		result[k] = v
	}
	return result
}
