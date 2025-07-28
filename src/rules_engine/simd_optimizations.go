package rules_engine

import (
	"unsafe"
)

// SIMDStringOperations provides SIMD-optimized string operations
// This is a proposal for SIMD optimization in the rules engine
type SIMDStringOperations struct {
	// CPU capability flags - platform specific
	hasAVX2   bool // x86_64 only
	hasSSE42  bool // x86_64 only
	hasSSSE3  bool // x86_64 only
	hasNEON   bool // ARM64 only
	hasCrypto bool // ARM64 crypto extensions
}

// NewSIMDStringOperations creates a new SIMD string operations instance
// with CPU capability detection
func NewSIMDStringOperations() *SIMDStringOperations {
	ops := &SIMDStringOperations{}
	ops.initPlatformSpecific()
	return ops
}

// SIMDContains performs SIMD-optimized string contains operation
func (s *SIMDStringOperations) SIMDContains(data, pattern string) bool {
	// Handle edge cases
	if len(pattern) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(pattern) > len(data) {
		return false
	}

	// Use platform-specific optimizations when available
	return s.platformOptimizedContains(data, pattern)
}

// SIMDContainsCaseInsensitive performs case-insensitive contains with SIMD
func (s *SIMDStringOperations) SIMDContainsCaseInsensitive(data, pattern string) bool {
	// Handle edge cases
	if len(pattern) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(pattern) > len(data) {
		return false
	}

	// Use platform-specific optimizations when available
	return s.platformOptimizedContainsCaseInsensitive(data, pattern)
}

// SIMDHasPrefix performs SIMD-optimized prefix checking
func (s *SIMDStringOperations) SIMDHasPrefix(data, prefix string) bool {
	// Handle edge cases
	if len(prefix) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(prefix) > len(data) {
		return false
	}

	// For prefix checking, we only need to compare the beginning
	return s.platformOptimizedContains(data[:len(prefix)], prefix)
}

// SIMDHasPrefixCaseInsensitive performs case-insensitive SIMD prefix checking
func (s *SIMDStringOperations) SIMDHasPrefixCaseInsensitive(data, prefix string) bool {
	// Handle edge cases
	if len(prefix) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(prefix) > len(data) {
		return false
	}

	// For prefix checking, we only need to compare the beginning
	return s.platformOptimizedContainsCaseInsensitive(data[:len(prefix)], prefix)
}

// SIMDHasSuffix performs SIMD-optimized suffix checking
func (s *SIMDStringOperations) SIMDHasSuffix(data, suffix string) bool {
	// Handle edge cases
	if len(suffix) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(suffix) > len(data) {
		return false
	}

	// For suffix checking, we only need to compare the end
	return s.platformOptimizedContains(data[len(data)-len(suffix):], suffix)
}

// SIMDHasSuffixCaseInsensitive performs case-insensitive SIMD suffix checking
func (s *SIMDStringOperations) SIMDHasSuffixCaseInsensitive(data, suffix string) bool {
	// Handle edge cases
	if len(suffix) == 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	if len(suffix) > len(data) {
		return false
	}

	// For suffix checking, we only need to compare the end
	return s.platformOptimizedContainsCaseInsensitive(data[len(data)-len(suffix):], suffix)
}

// BatchStringCompare performs multiple string comparisons in parallel using SIMD
func (s *SIMDStringOperations) BatchStringCompare(data string, patterns []string, operation CompareOperation) []bool {
	// Handle edge cases
	if len(patterns) == 0 {
		return []bool{}
	}
	if len(data) == 0 {
		// For empty data, only empty patterns can match (for contains operations)
		results := make([]bool, len(patterns))
		for i, pattern := range patterns {
			switch operation {
			case OpContains, OpContainsCaseInsensitive:
				results[i] = len(pattern) == 0
			case OpEquals:
				results[i] = pattern == ""
			case OpPrefix, OpSuffix, OpPrefixCaseInsensitive, OpSuffixCaseInsensitive:
				results[i] = len(pattern) == 0
			default:
				results[i] = false
			}
		}
		return results
	}

	// Use platform-specific batch operations
	return s.platformOptimizedBatchCompare(data, patterns, operation)
}

// CompareOperation defines the type of string comparison
type CompareOperation int

const (
	OpContains CompareOperation = iota
	OpContainsCaseInsensitive
	OpEquals
	OpPrefix
	OpSuffix
	OpPrefixCaseInsensitive
	OpSuffixCaseInsensitive
)

// SIMD implementation functions are declared in platform-specific files:
// - simd_amd64.go for x86_64 platforms
// - simd_arm64.go for ARM64 platforms
// - simd_generic.go for other platforms

// Fallback implementations
// Generic fallback implementations used by all platforms

// genericContains provides a fast generic string contains implementation
func (s *SIMDStringOperations) genericContains(data, pattern string) bool {
	return standardContains(data, pattern)
}

// genericContainsCaseInsensitive provides a fast generic case-insensitive contains
func (s *SIMDStringOperations) genericContainsCaseInsensitive(data, pattern string) bool {
	return standardContainsCaseInsensitive(data, pattern)
}

func standardContains(data, pattern string) bool {
	// Edge cases already handled by caller, but double-check for safety
	if len(pattern) == 0 {
		return true
	}
	if len(data) == 0 || len(pattern) > len(data) {
		return false
	}

	// Use unsafe for better performance
	dataBytes := *(*[]byte)(unsafe.Pointer(&data))
	patternBytes := *(*[]byte)(unsafe.Pointer(&pattern))

	// Boyer-Moore-like optimization for single character patterns
	if len(pattern) == 1 {
		patternByte := patternBytes[0]
		for _, b := range dataBytes {
			if b == patternByte {
				return true
			}
		}
		return false
	}

	// Use substring search for complex patterns
	for i := 0; i <= len(dataBytes)-len(patternBytes); i++ {
		match := true
		for j := 0; j < len(patternBytes); j++ {
			if dataBytes[i+j] != patternBytes[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func standardContainsCaseInsensitive(data, pattern string) bool {
	// Edge cases already handled by caller, but double-check for safety
	if len(pattern) == 0 {
		return true
	}
	if len(data) == 0 || len(pattern) > len(data) {
		return false
	}

	// Convert to bytes for faster comparison
	dataBytes := *(*[]byte)(unsafe.Pointer(&data))
	patternBytes := *(*[]byte)(unsafe.Pointer(&pattern))

	for i := 0; i <= len(dataBytes)-len(patternBytes); i++ {
		match := true
		for j := 0; j < len(patternBytes); j++ {
			if toLower(dataBytes[i+j]) != toLower(patternBytes[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Fast lowercase conversion using lookup table
var toLowerTable = [256]byte{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
	16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47,
	48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63,
	64, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111,
	112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 91, 92, 93, 94, 95,
	96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111,
	112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127,
	128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143,
	144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159,
	160, 161, 162, 163, 164, 165, 166, 167, 168, 169, 170, 171, 172, 173, 174, 175,
	176, 177, 178, 179, 180, 181, 182, 183, 184, 185, 186, 187, 188, 189, 190, 191,
	192, 193, 194, 195, 196, 197, 198, 199, 200, 201, 202, 203, 204, 205, 206, 207,
	208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219, 220, 221, 222, 223,
	224, 225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 235, 236, 237, 238, 239,
	240, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 251, 252, 253, 254, 255,
}

func toLower(b byte) byte {
	return toLowerTable[b]
}

// Integration points for the existing rule engine

// SIMDOptimizedNCS_INCL is a SIMD-optimized version of NCS_INCL
func SIMDOptimizedNCS_INCL(simdOps *SIMDStringOperations, data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	return simdOps.SIMDContainsCaseInsensitive(data, ruleData), ruleData
}

// SIMDOptimizedORLogic performs OR logic with SIMD batch operations
func SIMDOptimizedORLogic(simdOps *SIMDStringOperations, data string, patterns []string, operation CompareOperation) bool {
	results := simdOps.BatchStringCompare(data, patterns, operation)
	for _, result := range results {
		if result {
			return true
		}
	}
	return false
}

// SIMDOptimizedANDLogic performs AND logic with SIMD batch operations
func SIMDOptimizedANDLogic(simdOps *SIMDStringOperations, data string, patterns []string, operation CompareOperation) bool {
	results := simdOps.BatchStringCompare(data, patterns, operation)
	for _, result := range results {
		if !result {
			return false
		}
	}
	return true
}
