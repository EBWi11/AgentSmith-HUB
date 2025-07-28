//go:build amd64
// +build amd64

package rules_engine

import (
	"golang.org/x/sys/cpu"
)

// CPU capability detection using Go's cpu package
func detectAVX2() bool {
	return cpu.X86.HasAVX2
}

func detectSSE42() bool {
	return cpu.X86.HasSSE42
}

func detectSSSE3() bool {
	return cpu.X86.HasSSSE3
}

// initPlatformSpecific initializes x86_64 specific SIMD capabilities
func (ops *SIMDStringOperations) initPlatformSpecific() {
	ops.hasAVX2 = detectAVX2()
	ops.hasSSE42 = detectSSE42()
	ops.hasSSSE3 = detectSSSE3()
	ops.hasNEON = false // Not available on x86_64
	ops.hasCrypto = false
}

// x86_64-optimized implementations
func (ops *SIMDStringOperations) platformOptimizedContains(data, pattern string) bool {
	// Additional edge case check (main function already handles basic cases)
	if len(pattern) > len(data) {
		return false
	}

	// Use AVX2 for longer strings
	if len(data) >= 32 && len(pattern) >= 4 && ops.hasAVX2 {
		return ops.avx2Contains(data, pattern)
	} else if len(data) >= 16 && len(pattern) >= 4 && ops.hasSSE42 {
		return ops.sse42Contains(data, pattern)
	}

	return ops.genericContains(data, pattern)
}

func (ops *SIMDStringOperations) platformOptimizedContainsCaseInsensitive(data, pattern string) bool {
	// Additional edge case check (main function already handles basic cases)
	if len(pattern) > len(data) {
		return false
	}

	// Use AVX2 for longer strings
	if len(data) >= 32 && len(pattern) >= 4 && ops.hasAVX2 {
		return ops.avx2ContainsCaseInsensitive(data, pattern)
	} else if len(data) >= 16 && len(pattern) >= 4 && ops.hasSSE42 {
		return ops.sse42ContainsCaseInsensitive(data, pattern)
	}

	return ops.genericContainsCaseInsensitive(data, pattern)
}

func (ops *SIMDStringOperations) platformOptimizedBatchCompare(data string, patterns []string, operation CompareOperation) []bool {
	// Use SIMD for batch operations when we have multiple patterns
	if len(patterns) >= 4 && ops.hasAVX2 {
		return ops.avx2BatchCompare(data, patterns, operation)
	} else if len(patterns) >= 2 && ops.hasSSE42 {
		return ops.sse42BatchCompare(data, patterns, operation)
	}

	// Fallback to generic processing
	results := make([]bool, len(patterns))
	for i, pattern := range patterns {
		switch operation {
		case OpContains:
			results[i] = ops.genericContains(data, pattern)
		case OpContainsCaseInsensitive:
			results[i] = ops.genericContainsCaseInsensitive(data, pattern)
		case OpEquals:
			results[i] = data == pattern
		case OpPrefix:
			results[i] = len(data) >= len(pattern) && data[:len(pattern)] == pattern
		case OpSuffix:
			results[i] = len(data) >= len(pattern) && data[len(data)-len(pattern):] == pattern
		case OpPrefixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.genericContainsCaseInsensitive(data[:len(pattern)], pattern)
			} else {
				results[i] = false
			}
		case OpSuffixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.genericContainsCaseInsensitive(data[len(data)-len(pattern):], pattern)
			} else {
				results[i] = false
			}
		default:
			results[i] = ops.genericContains(data, pattern)
		}
	}
	return results
}

// AVX2-optimized implementations (placeholder for future assembly optimization)
func (ops *SIMDStringOperations) avx2Contains(data, pattern string) bool {
	// TODO: Implement AVX2 assembly optimizations
	// For now, use optimized Go implementation
	return ops.genericContains(data, pattern)
}

func (ops *SIMDStringOperations) avx2ContainsCaseInsensitive(data, pattern string) bool {
	// TODO: Implement AVX2 assembly optimizations
	return ops.genericContainsCaseInsensitive(data, pattern)
}

func (ops *SIMDStringOperations) avx2BatchCompare(data string, patterns []string, operation CompareOperation) []bool {
	// TODO: Implement AVX2 batch processing
	// For now, use optimized Go implementation with batch processing
	results := make([]bool, len(patterns))
	for i, pattern := range patterns {
		switch operation {
		case OpContains:
			results[i] = ops.avx2Contains(data, pattern)
		case OpContainsCaseInsensitive:
			results[i] = ops.avx2ContainsCaseInsensitive(data, pattern)
		case OpEquals:
			results[i] = data == pattern
		case OpPrefix:
			results[i] = len(data) >= len(pattern) && data[:len(pattern)] == pattern
		case OpSuffix:
			results[i] = len(data) >= len(pattern) && data[len(data)-len(pattern):] == pattern
		case OpPrefixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.avx2ContainsCaseInsensitive(data[:len(pattern)], pattern)
			} else {
				results[i] = false
			}
		case OpSuffixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.avx2ContainsCaseInsensitive(data[len(data)-len(pattern):], pattern)
			} else {
				results[i] = false
			}
		default:
			results[i] = ops.genericContains(data, pattern)
		}
	}
	return results
}

// SSE4.2-optimized implementations (placeholder for future assembly optimization)
func (ops *SIMDStringOperations) sse42Contains(data, pattern string) bool {
	// TODO: Implement SSE4.2 assembly optimizations
	return ops.genericContains(data, pattern)
}

func (ops *SIMDStringOperations) sse42ContainsCaseInsensitive(data, pattern string) bool {
	// TODO: Implement SSE4.2 assembly optimizations
	return ops.genericContainsCaseInsensitive(data, pattern)
}

func (ops *SIMDStringOperations) sse42BatchCompare(data string, patterns []string, operation CompareOperation) []bool {
	// TODO: Implement SSE4.2 batch processing
	// For now, use optimized Go implementation with batch processing
	results := make([]bool, len(patterns))
	for i, pattern := range patterns {
		switch operation {
		case OpContains:
			results[i] = ops.sse42Contains(data, pattern)
		case OpContainsCaseInsensitive:
			results[i] = ops.sse42ContainsCaseInsensitive(data, pattern)
		case OpEquals:
			results[i] = data == pattern
		case OpPrefix:
			results[i] = len(data) >= len(pattern) && data[:len(pattern)] == pattern
		case OpSuffix:
			results[i] = len(data) >= len(pattern) && data[len(data)-len(pattern):] == pattern
		case OpPrefixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.sse42ContainsCaseInsensitive(data[:len(pattern)], pattern)
			} else {
				results[i] = false
			}
		case OpSuffixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = ops.sse42ContainsCaseInsensitive(data[len(data)-len(pattern):], pattern)
			} else {
				results[i] = false
			}
		default:
			results[i] = ops.genericContains(data, pattern)
		}
	}
	return results
}
