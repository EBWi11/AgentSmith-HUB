package common

import (
	"testing"
	"time"
)

func TestSampler(t *testing.T) {
	// 创建采样器
	sampler := NewSampler("test")
	if sampler == nil {
		t.Fatal("Failed to create sampler")
	}
	defer sampler.Close()

	// 测试基本采样功能
	testData := map[string]interface{}{
		"message": "test message",
		"id":      123,
	}

	// 由于采样率是千分之一，我们需要发送足够多的数据才能有采样结果
	sampleCount := 0
	for i := 0; i < 10000; i++ {
		if sampler.Sample(testData, "test", "test-project") {
			sampleCount++
		}
	}

	// 检查统计信息
	stats := sampler.GetStats()
	if stats.TotalCount != 10000 {
		t.Errorf("Expected total count 10000, got %d", stats.TotalCount)
	}

	if stats.SampledCount == 0 {
		t.Error("Expected some samples, got 0")
	}

	// 采样率应该大约是千分之一（允许一些误差）
	actualRate := float64(stats.SampledCount) / float64(stats.TotalCount)
	expectedRate := 0.001
	if actualRate < expectedRate*0.5 || actualRate > expectedRate*2 {
		t.Errorf("Expected sampling rate around %f, got %f", expectedRate, actualRate)
	}

	// 等待异步处理完成
	time.Sleep(100 * time.Millisecond)

	// 检查采样数据
	samples := sampler.GetSamplesByProject("test-project")
	if len(samples) == 0 {
		t.Error("Expected some sample data, got none")
	}

	// 验证采样数据结构
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
	// 测试空参数
	sampler := NewSampler("test")
	defer sampler.Close()

	// 空数据应该返回false
	if sampler.Sample(nil, "test", "test-project") {
		t.Error("Expected false for nil data")
	}

	// 空source应该返回false
	if sampler.Sample("data", "", "test-project") {
		t.Error("Expected false for empty source")
	}

	// 空projectNodeSequence应该返回false
	if sampler.Sample("data", "test", "") {
		t.Error("Expected false for empty project node sequence")
	}
}

func TestSamplerClose(t *testing.T) {
	sampler := NewSampler("test")

	// 关闭采样器
	sampler.Close()

	// 关闭后应该拒绝新的采样
	if sampler.Sample("data", "test", "test-project") {
		t.Error("Expected false after sampler is closed")
	}
}

func TestGetSampler(t *testing.T) {
	// 测试空名称
	sampler := GetSampler("")
	if sampler != nil {
		t.Error("Expected nil for empty name")
	}

	// 测试正常获取
	sampler1 := GetSampler("test1")
	if sampler1 == nil {
		t.Error("Expected non-nil sampler")
	}

	// 再次获取应该返回同一个实例
	sampler2 := GetSampler("test1")
	if sampler1 != sampler2 {
		t.Error("Expected same sampler instance")
	}

	// 清理
	sampler1.Close()
}
