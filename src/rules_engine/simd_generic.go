//go:build !amd64 && !arm64
// +build !amd64,!arm64

package rules_engine

// initPlatformSpecific initializes non-SIMD platforms (fallback)
func (ops *SIMDStringOperations) initPlatformSpecific() {
	ops.hasAVX2 = false
	ops.hasSSE42 = false
	ops.hasSSSE3 = false
	ops.hasNEON = false
	ops.hasCrypto = false
}

// Generic implementations for non-SIMD platforms
func (ops *SIMDStringOperations) platformOptimizedContains(data, pattern string) bool {
	return ops.genericContains(data, pattern)
}

func (ops *SIMDStringOperations) platformOptimizedContainsCaseInsensitive(data, pattern string) bool {
	return ops.genericContainsCaseInsensitive(data, pattern)
}

func (ops *SIMDStringOperations) platformOptimizedBatchCompare(data string, patterns []string, operation CompareOperation) []bool {
	return ops.genericBatchCompare(data, patterns, operation)
}

// Generic batch compare for non-SIMD platforms
func (s *SIMDStringOperations) genericBatchCompare(data string, patterns []string, operation CompareOperation) []bool {
	results := make([]bool, len(patterns))

	for i, pattern := range patterns {
		switch operation {
		case OpContains:
			results[i] = standardContains(data, pattern)
		case OpContainsCaseInsensitive:
			results[i] = standardContainsCaseInsensitive(data, pattern)
		case OpEquals:
			results[i] = data == pattern
		case OpPrefix:
			results[i] = len(data) >= len(pattern) && data[:len(pattern)] == pattern
		case OpSuffix:
			results[i] = len(data) >= len(pattern) && data[len(data)-len(pattern):] == pattern
		case OpPrefixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = standardContainsCaseInsensitive(data[:len(pattern)], pattern)
			} else {
				results[i] = false
			}
		case OpSuffixCaseInsensitive:
			if len(data) >= len(pattern) {
				results[i] = standardContainsCaseInsensitive(data[len(data)-len(pattern):], pattern)
			} else {
				results[i] = false
			}
		}
	}

	return results
}
