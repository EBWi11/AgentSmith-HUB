package common

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

const (
	// 使用位运算优化采样率计算
	SamplingMask = 1023 // 2^10 - 1，对应千分之一的采样率
)

// SampleData represents a single sample with its metadata
type SampleData struct {
	Data                interface{} `json:"data"`
	Timestamp           time.Time   `json:"timestamp"`
	Source              string      `json:"source"`
	ProjectNodeSequence string      `json:"project_node_sequence"`
}

// ProjectSamples 使用无锁环形缓冲区
type ProjectSamples struct {
	samples     [100]SampleData // 固定大小的环形缓冲区
	writeIdx    uint32          // 写入位置
	sampleCount uint32          // 当前样本数量
	mu          sync.RWMutex    // 读写锁保护数据一致性
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
	pool         *ants.Pool // 用于异步处理采样数据
	closed       int32      // 标记是否已关闭
}

// NewSampler creates a new sampler instance
func NewSampler(name string) *Sampler {
	pool, err := ants.NewPool(8, ants.WithPreAlloc(true))
	if err != nil {
		// 如果创建协程池失败，使用默认池
		pool = nil
	}
	return &Sampler{
		name:       name,
		maxSamples: 100,
		pool:       pool,
	}
}

// getOrCreateProjectSamples 获取或创建项目采样器
func (s *Sampler) getOrCreateProjectSamples(projectNodeSequence string) *ProjectSamples {
	value, _ := s.samples.LoadOrStore(projectNodeSequence, &ProjectSamples{})
	return value.(*ProjectSamples)
}

// Sample attempts to sample the data based on sampling rate
func (s *Sampler) Sample(data interface{}, source string, projectNodeSequence string) bool {
	// 检查是否已关闭
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}

	// 检查参数有效性
	if data == nil || source == "" || projectNodeSequence == "" {
		return false
	}

	// 增加计数器，使用原子操作
	total := atomic.AddUint64(&s.totalCount, 1)

	// 使用位运算快速判断是否需要采样
	// 这里使用 total & SamplingMask 代替随机数，可以保证均匀分布
	if total&SamplingMask != 0 {
		return false
	}

	// 增加采样计数
	atomic.AddUint64(&s.sampledCount, 1)

	// 创建采样数据
	sample := SampleData{
		Data:                data,
		Timestamp:           time.Now(),
		Source:              source,
		ProjectNodeSequence: projectNodeSequence,
	}

	// 如果有协程池，异步处理；否则同步处理
	if s.pool != nil {
		// 检查协程池是否已关闭
		if s.pool.IsClosed() {
			s.storeSample(sample, projectNodeSequence)
		} else {
			err := s.pool.Submit(func() {
				s.storeSample(sample, projectNodeSequence)
			})
			if err != nil {
				// 如果提交失败，同步处理
				s.storeSample(sample, projectNodeSequence)
			}
		}
	} else {
		s.storeSample(sample, projectNodeSequence)
	}

	return true
}

// storeSample 存储采样数据到环形缓冲区
func (s *Sampler) storeSample(sample SampleData, projectNodeSequence string) {
	// 获取项目采样器
	ps := s.getOrCreateProjectSamples(projectNodeSequence)

	// 使用写锁保护写入操作
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 写入采样数据到环形缓冲区
	writeIdx := ps.writeIdx % 100
	ps.samples[writeIdx] = sample
	ps.writeIdx++

	// 更新样本数量
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

		// 使用读锁保护读取操作
		ps.mu.RLock()
		count := ps.sampleCount
		writeIdx := ps.writeIdx

		if count == 0 {
			ps.mu.RUnlock()
			return true
		}

		// 复制样本数据
		samples := make([]SampleData, count)

		// 从最旧的数据开始复制（如果缓冲区满了）
		if count == 100 {
			// 缓冲区已满，从最旧的数据开始
			startIdx := writeIdx % 100
			for i := uint32(0); i < count; i++ {
				idx := (startIdx + i) % 100
				samples[i] = ps.samples[idx]
			}
		} else {
			// 缓冲区未满，从索引0开始
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

		// 使用读锁保护读取操作
		ps.mu.RLock()
		defer ps.mu.RUnlock()

		count := ps.sampleCount
		writeIdx := ps.writeIdx

		if count == 0 {
			return nil
		}

		// 复制样本数据
		samples := make([]SampleData, count)

		// 从最旧的数据开始复制（如果缓冲区满了）
		if count == 100 {
			// 缓冲区已满，从最旧的数据开始
			startIdx := writeIdx % 100
			for i := uint32(0); i < count; i++ {
				idx := (startIdx + i) % 100
				samples[i] = ps.samples[idx]
			}
		} else {
			// 缓冲区未满，从索引0开始
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
		SamplingRate:   0.001,
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
	// 标记为已关闭
	atomic.StoreInt32(&s.closed, 1)

	// 关闭协程池
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
