package rules_engine

import (
	"AgentSmith-HUB/common"
	"sync"
)

// Global SIMD operations instance with lazy initialization
var (
	globalSIMDOps *SIMDStringOperations
	simdOnce      sync.Once
)

// GetSIMDOperations returns the global SIMD operations instance
func GetSIMDOperations() *SIMDStringOperations {
	simdOnce.Do(func() {
		globalSIMDOps = NewSIMDStringOperations()
	})
	return globalSIMDOps
}

// SIMDEnhancedNCS_INCL provides SIMD-optimized version of NCS_INCL
func SIMDEnhancedNCS_INCL(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDContainsCaseInsensitive(data, ruleData), ruleData
}

// SIMDEnhancedNCS_START provides SIMD-optimized version of NCS_START
func SIMDEnhancedNCS_START(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDHasPrefixCaseInsensitive(data, ruleData), ruleData
}

// SIMDEnhancedINCL provides SIMD-optimized version of INCL
func SIMDEnhancedINCL(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDContains(data, ruleData), ruleData
}

// SIMDEnhancedSTART provides SIMD-optimized version of START
func SIMDEnhancedSTART(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDHasPrefix(data, ruleData), ruleData
}

// SIMDEnhancedEND provides SIMD-optimized version of END
func SIMDEnhancedEND(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDHasSuffix(data, ruleData), ruleData
}

// SIMDEnhancedNCS_END provides SIMD-optimized version of NCS_END
func SIMDEnhancedNCS_END(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}
	if data == "" {
		return false, ""
	}

	simdOps := GetSIMDOperations()
	return simdOps.SIMDHasSuffixCaseInsensitive(data, ruleData), ruleData
}

// SIMDEnhancedExecuteCheckNode optimizes the check node execution with SIMD
func (r *Ruleset) SIMDEnhancedExecuteCheckNode(checkNode *CheckNodes, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	// Handle OR logic with SIMD batch operations
	if checkNode.Logic == "OR" && len(checkNode.DelimiterFieldList) > 1 {
		return r.simdExecuteORLogic(checkNode, data, ruleCache)
	}

	// Handle AND logic with SIMD batch operations
	if checkNode.Logic == "AND" && len(checkNode.DelimiterFieldList) > 1 {
		return r.simdExecuteANDLogic(checkNode, data, ruleCache)
	}

	// Fallback to original implementation for single checks
	return r.executeCheckNode(checkNode, data, ruleCache)
}

// simdExecuteORLogic uses SIMD to process OR logic efficiently
func (r *Ruleset) simdExecuteORLogic(checkNode *CheckNodes, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	needCheckData, exist := common.GetCheckData(data, checkNode.FieldList)
	if !exist {
		return false
	}

	simdOps := GetSIMDOperations()

	// Prepare patterns for batch processing
	patterns := make([]string, 0, len(checkNode.DelimiterFieldList))
	for _, v := range checkNode.DelimiterFieldList {
		var checkNodeValue string
		if hasFromRawPrefix(v) {
			checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
		} else {
			checkNodeValue = v
		}
		patterns = append(patterns, checkNodeValue)
	}

	// Choose the appropriate SIMD operation based on check type
	var operation CompareOperation
	switch checkNode.Type {
	case "INCL":
		operation = OpContains
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_INCL":
		operation = OpContainsCaseInsensitive
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "EQU", "NCS_EQU":
		operation = OpEquals
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "START":
		operation = OpPrefix
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_START":
		operation = OpPrefixCaseInsensitive
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "END":
		operation = OpSuffix
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_END":
		operation = OpSuffixCaseInsensitive
		return SIMDOptimizedORLogic(simdOps, needCheckData, patterns, operation)

	default:
		// Fallback to sequential processing for unsupported types
		return r.fallbackORLogic(checkNode, needCheckData, patterns, ruleCache)
	}
}

// simdExecuteANDLogic uses SIMD to process AND logic efficiently
func (r *Ruleset) simdExecuteANDLogic(checkNode *CheckNodes, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	needCheckData, exist := common.GetCheckData(data, checkNode.FieldList)
	if !exist {
		return false
	}

	simdOps := GetSIMDOperations()

	// Prepare patterns for batch processing
	patterns := make([]string, 0, len(checkNode.DelimiterFieldList))
	for _, v := range checkNode.DelimiterFieldList {
		var checkNodeValue string
		if hasFromRawPrefix(v) {
			checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
		} else {
			checkNodeValue = v
		}
		patterns = append(patterns, checkNodeValue)
	}

	// Choose the appropriate SIMD operation based on check type
	var operation CompareOperation
	switch checkNode.Type {
	case "INCL":
		operation = OpContains
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_INCL":
		operation = OpContainsCaseInsensitive
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "EQU", "NCS_EQU":
		operation = OpEquals
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "START":
		operation = OpPrefix
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_START":
		operation = OpPrefixCaseInsensitive
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "END":
		operation = OpSuffix
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	case "NCS_END":
		operation = OpSuffixCaseInsensitive
		return SIMDOptimizedANDLogic(simdOps, needCheckData, patterns, operation)

	default:
		// Fallback to sequential processing for unsupported types
		return r.fallbackANDLogic(checkNode, needCheckData, patterns, ruleCache)
	}
}

// fallbackORLogic handles cases where SIMD optimization is not applicable
func (r *Ruleset) fallbackORLogic(checkNode *CheckNodes, needCheckData string, patterns []string, ruleCache map[string]common.CheckCoreCache) bool {
	for _, pattern := range patterns {
		result, _ := checkNode.CheckFunc(needCheckData, pattern)
		if result {
			return true
		}
	}
	return false
}

// fallbackANDLogic handles cases where SIMD optimization is not applicable
func (r *Ruleset) fallbackANDLogic(checkNode *CheckNodes, needCheckData string, patterns []string, ruleCache map[string]common.CheckCoreCache) bool {
	for _, pattern := range patterns {
		result, _ := checkNode.CheckFunc(needCheckData, pattern)
		if !result {
			return false
		}
	}
	return true
}

// Performance monitoring for SIMD operations
type SIMDPerformanceStats struct {
	SIMDOperationsCount     uint64
	FallbackOperationsCount uint64
	AverageSpeedup          float64
	mutex                   sync.RWMutex
}

var globalSIMDStats = &SIMDPerformanceStats{}

// GetSIMDPerformanceStats returns current SIMD performance statistics
func GetSIMDPerformanceStats() SIMDPerformanceStats {
	globalSIMDStats.mutex.RLock()
	defer globalSIMDStats.mutex.RUnlock()
	return SIMDPerformanceStats{
		SIMDOperationsCount:     globalSIMDStats.SIMDOperationsCount,
		FallbackOperationsCount: globalSIMDStats.FallbackOperationsCount,
		AverageSpeedup:          globalSIMDStats.AverageSpeedup,
	}
}

// Configuration for SIMD optimization thresholds
type SIMDConfig struct {
	MinStringLengthForSIMD int  // Minimum string length to use SIMD (default: 16)
	MinPatternsForBatch    int  // Minimum patterns for batch processing (default: 4)
	EnableAVX2             bool // Enable AVX2 optimizations (default: auto-detect)
	EnableSSE42            bool // Enable SSE4.2 optimizations (default: auto-detect)
}

var defaultSIMDConfig = SIMDConfig{
	MinStringLengthForSIMD: 16,
	MinPatternsForBatch:    4,
	EnableAVX2:             true,
	EnableSSE42:            true,
}

// SetSIMDConfig allows customization of SIMD optimization behavior
func SetSIMDConfig(config SIMDConfig) {
	defaultSIMDConfig = config
}

// GetSIMDConfig returns the current SIMD configuration
func GetSIMDConfig() SIMDConfig {
	return defaultSIMDConfig
}
