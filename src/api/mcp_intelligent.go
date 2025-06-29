package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// =====================================================
// INTELLIGENT DATA STRUCTURES
// =====================================================

// IntelligentSampleDataRequest represents enhanced sample data request
type IntelligentSampleDataRequest struct {
	// Backward compatibility
	SamplerType string `json:"sampler_type,omitempty"`
	Count       int    `json:"count,omitempty"`

	// New intelligent parameters
	TargetProjects    []string `json:"target_projects,omitempty"`
	RulePurpose       string   `json:"rule_purpose,omitempty"`
	FieldRequirements []string `json:"field_requirements,omitempty"`
	QualityThreshold  float64  `json:"quality_threshold,omitempty"`
}

// IntelligentSampleDataResponse represents enhanced response with analysis
type IntelligentSampleDataResponse struct {
	SampleData      map[string]interface{} `json:"sample_data"`
	DataQuality     DataQualityAnalysis    `json:"data_quality"`
	FieldAnalysis   FieldUsageAnalysis     `json:"field_analysis"`
	Recommendations []string               `json:"recommendations"`
	ProjectContext  ProjectContextInfo     `json:"project_context"`
}

type DataQualityAnalysis struct {
	OverallScore      float64        `json:"overall_score"`
	FieldCoverage     float64        `json:"field_coverage"`
	DataFreshness     string         `json:"data_freshness"`
	VolumeEstimate    int64          `json:"volume_estimate"`
	QualityIssues     []string       `json:"quality_issues"`
	FieldDistribution map[string]int `json:"field_distribution"`
}

type FieldUsageAnalysis struct {
	AvailableFields    []string                   `json:"available_fields"`
	RecommendedFields  []string                   `json:"recommended_fields"`
	FieldTypes         map[string]string          `json:"field_types"`
	ValueDistributions map[string]FieldValueStats `json:"value_distributions"`
	PerformanceImpact  map[string]string          `json:"performance_impact"`
}

type FieldValueStats struct {
	UniqueValues   int              `json:"unique_values"`
	TopValues      []ValueFrequency `json:"top_values"`
	DataType       string           `json:"data_type"`
	NullPercentage float64          `json:"null_percentage"`
}

type ValueFrequency struct {
	Value      interface{} `json:"value"`
	Count      int         `json:"count"`
	Percentage float64     `json:"percentage"`
}

type ProjectContextInfo struct {
	TargetProjects    []ProjectProfile `json:"target_projects"`
	SuggestedProjects []ProjectProfile `json:"suggested_projects"`
	DataSources       []string         `json:"data_sources"`
	CommonFields      []string         `json:"common_fields"`
	TotalProjects     int              `json:"total_projects"`
}

type ProjectProfile struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Status          string             `json:"status"`
	DataVolume      int64              `json:"data_volume"`
	FieldUsage      map[string]float64 `json:"field_usage"`
	ExistingRules   int                `json:"existing_rules"`
	PerformanceLoad string             `json:"performance_load"`
	Relevance       float64            `json:"relevance"`
	Reasoning       string             `json:"reasoning"`
}

// =====================================================
// INTELLIGENT SAMPLE DATA API
// =====================================================

// GetSamplersDataIntelligent - Enhanced version with context awareness
func GetSamplersDataIntelligent(c echo.Context) error {
	if !cluster.IsLeader {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "Sample data collection is only available on leader node",
			"data":    map[string]interface{}{},
		})
	}

	var req IntelligentSampleDataRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
	}

	logger.Info("Intelligent sample data request",
		"targetProjects", req.TargetProjects,
		"rulePurpose", req.RulePurpose,
		"fieldRequirements", req.FieldRequirements)

	// Backward compatibility fallback
	if len(req.TargetProjects) == 0 && req.RulePurpose == "" && len(req.FieldRequirements) == 0 {
		logger.Info("Falling back to basic sample data API")
		return GetSamplerData(c)
	}

	// Extract target ruleset from field requirements or rule purpose
	targetRuleset := extractTargetRulesetFromRequest(req)
	if targetRuleset == "" {
		logger.Info("No target ruleset identified, falling back to basic API")
		return GetSamplerData(c)
	}

	logger.Info("Target ruleset identified", "ruleset", targetRuleset)

	// Get sample data for the target ruleset
	sampleData, dataSource, err := getSampleDataForRuleset(targetRuleset)
	if err != nil {
		logger.Error("Failed to get sample data for ruleset", "error", err, "ruleset", targetRuleset)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Data fetching failed"})
	}

	// Analyze data quality and generate insights
	dataQuality := analyzeDataQuality(sampleData, req.FieldRequirements)
	fieldAnalysis := analyzeFieldUsage(sampleData, req.RulePurpose)
	recommendations := generateDataRecommendations(dataQuality, fieldAnalysis, ProjectContextInfo{})

	response := IntelligentSampleDataResponse{
		SampleData:      sampleData,
		DataQuality:     dataQuality,
		FieldAnalysis:   fieldAnalysis,
		Recommendations: recommendations,
		ProjectContext: ProjectContextInfo{
			DataSources: []string{dataSource},
		},
	}

	logger.Info("Intelligent sample data response ready",
		"targetRuleset", targetRuleset,
		"dataSource", dataSource,
		"dataQualityScore", dataQuality.OverallScore)

	return c.JSON(http.StatusOK, response)
}

// =====================================================
// PROJECT CONTEXT ANALYSIS
// =====================================================

func analyzeProjectContext(targetProjects []string, rulePurpose string) (ProjectContextInfo, error) {
	context := ProjectContextInfo{
		TargetProjects:    make([]ProjectProfile, 0),
		SuggestedProjects: make([]ProjectProfile, 0),
		DataSources:       make([]string, 0),
		CommonFields:      make([]string, 0),
	}

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	allProjects := project.GlobalProject.Projects
	context.TotalProjects = len(allProjects)

	// Auto-suggest projects if none specified
	if len(targetProjects) == 0 {
		targetProjects = suggestProjectsBasedOnPurpose(rulePurpose, allProjects)
		logger.Info("Auto-suggested projects", "suggested", targetProjects)
	}

	// Analyze target projects
	for _, projectID := range targetProjects {
		if proj, exists := allProjects[projectID]; exists {
			profile := analyzeProjectProfile(proj, rulePurpose)
			context.TargetProjects = append(context.TargetProjects, profile)

			// Collect data sources
			for inputID := range proj.Inputs {
				context.DataSources = append(context.DataSources, fmt.Sprintf("input.%s", inputID))
			}
			for rulesetID := range proj.Rulesets {
				context.DataSources = append(context.DataSources, fmt.Sprintf("ruleset.%s", rulesetID))
			}
		}
	}

	// Suggest additional relevant projects
	for projectID, proj := range allProjects {
		if !contains(targetProjects, projectID) {
			relevance := calculateProjectRelevance(proj, rulePurpose)
			if relevance > 0.5 {
				profile := analyzeProjectProfile(proj, rulePurpose)
				profile.Relevance = relevance
				profile.Reasoning = generateRelevanceReasoning(proj, rulePurpose)
				context.SuggestedProjects = append(context.SuggestedProjects, profile)
			}
		}
	}

	// Sort and limit suggestions
	sort.Slice(context.SuggestedProjects, func(i, j int) bool {
		return context.SuggestedProjects[i].Relevance > context.SuggestedProjects[j].Relevance
	})
	if len(context.SuggestedProjects) > 3 {
		context.SuggestedProjects = context.SuggestedProjects[:3]
	}

	return context, nil
}

func analyzeProjectProfile(proj *project.Project, rulePurpose string) ProjectProfile {
	return ProjectProfile{
		ID:              proj.Id,
		Name:            proj.Id,
		Status:          string(proj.Status),
		DataVolume:      estimateProjectDataVolume(proj),
		FieldUsage:      make(map[string]float64),
		ExistingRules:   len(proj.Rulesets),
		PerformanceLoad: assessPerformanceLoad(proj),
		Relevance:       1.0,
		Reasoning:       "Target project specified by user",
	}
}

// =====================================================
// INTELLIGENT DATA FETCHING
// =====================================================

func fetchIntelligentSampleData(req IntelligentSampleDataRequest, context ProjectContextInfo) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// For rule creation scenarios, we need to find the target ruleset
	// and follow the data flow backwards
	if req.RulePurpose != "" {
		// Try to find target component from field requirements or context
		targetComponent := extractTargetComponent(req, context)
		if targetComponent != "" {
			logger.Info("Target component identified", "component", targetComponent)

			// Step 1: Try to get sample data from the target component itself
			componentData := getSampleDataFromComponent(targetComponent)
			if len(componentData) > 0 {
				logger.Info("Found sample data from target component", "component", targetComponent, "samples", len(componentData))
				result[targetComponent] = componentData
				return result, nil
			}

			// Step 2: If target component has no data, find projects using this component
			usingProjects := findProjectsUsingComponent(targetComponent)
			logger.Info("Projects using target component", "component", targetComponent, "projects", usingProjects)

			// Step 3: For each project, find upstream input components
			for _, projectID := range usingProjects {
				upstreamInputs := findUpstreamInputs(projectID, targetComponent)
				logger.Info("Upstream inputs found", "project", projectID, "inputs", upstreamInputs)

				// Step 4: Try to get sample data from upstream inputs
				for _, inputID := range upstreamInputs {
					inputData := getSampleDataFromComponent("input." + inputID)
					if len(inputData) > 0 {
						logger.Info("Found sample data from upstream input", "input", inputID, "samples", len(inputData))
						// Use project.component format for clarity
						key := fmt.Sprintf("%s.%s->%s", projectID, inputID, targetComponent)
						result[key] = inputData
						return result, nil
					}
				}
			}

			// Step 5: If still no data, return empty with clear message
			logger.Info("No sample data found in target component or upstream inputs", "component", targetComponent)
		}
	}

	// Fallback: Try to get any available sample data
	fallbackData := getFallbackSampleData()
	if len(fallbackData) > 0 {
		result["fallback"] = fallbackData
	}

	logger.Info("Intelligent sample data fetched", "totalSources", len(result))
	return result, nil
}

// =====================================================
// HELPER FUNCTIONS
// =====================================================

func suggestProjectsBasedOnPurpose(purpose string, projects map[string]*project.Project) []string {
	suggestions := make([]string, 0)
	purposeLower := strings.ToLower(purpose)

	for projectID := range projects {
		projectLower := strings.ToLower(projectID)
		if (strings.Contains(purposeLower, "network") && strings.Contains(projectLower, "net")) ||
			(strings.Contains(purposeLower, "security") && strings.Contains(projectLower, "sec")) ||
			(strings.Contains(purposeLower, "api") && strings.Contains(projectLower, "api")) {
			suggestions = append(suggestions, projectID)
		}
	}

	if len(suggestions) == 0 {
		count := 0
		for projectID := range projects {
			if count < 2 {
				suggestions = append(suggestions, projectID)
				count++
			}
		}
	}

	return suggestions
}

func calculateProjectRelevance(proj *project.Project, rulePurpose string) float64 {
	relevance := 0.0
	purposeLower := strings.ToLower(rulePurpose)
	projectLower := strings.ToLower(proj.Id)

	if strings.Contains(purposeLower, "network") && strings.Contains(projectLower, "net") {
		relevance += 0.4
	}
	if strings.Contains(purposeLower, "security") && strings.Contains(projectLower, "sec") {
		relevance += 0.4
	}
	if strings.Contains(purposeLower, "api") && strings.Contains(projectLower, "api") {
		relevance += 0.4
	}

	if relevance > 1.0 {
		return 1.0
	}
	return relevance
}

func generateRelevanceReasoning(proj *project.Project, rulePurpose string) string {
	reasons := make([]string, 0)
	purposeLower := strings.ToLower(rulePurpose)
	projectLower := strings.ToLower(proj.Id)

	if strings.Contains(purposeLower, "network") && strings.Contains(projectLower, "net") {
		reasons = append(reasons, "handles network data")
	}
	if strings.Contains(purposeLower, "security") && strings.Contains(projectLower, "sec") {
		reasons = append(reasons, "security focused")
	}
	if strings.Contains(purposeLower, "api") && strings.Contains(projectLower, "api") {
		reasons = append(reasons, "processes API traffic")
	}

	if len(reasons) == 0 {
		return "similar component structure"
	}
	return strings.Join(reasons, ", ")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func estimateProjectDataVolume(proj *project.Project) int64 {
	return int64((len(proj.Inputs) + len(proj.Rulesets) + len(proj.Outputs)) * 1000)
}

func assessPerformanceLoad(proj *project.Project) string {
	total := len(proj.Inputs) + len(proj.Rulesets) + len(proj.Outputs)
	if total > 10 {
		return "high"
	} else if total > 5 {
		return "medium"
	}
	return "low"
}

func getAllSamplerNames() []string {
	samplerNames := make([]string, 0)

	common.GlobalMu.RLock()
	for inputId := range project.GlobalProject.Inputs {
		samplerNames = append(samplerNames, "input."+inputId)
	}
	for rulesetId := range project.GlobalProject.Rulesets {
		samplerNames = append(samplerNames, "ruleset."+rulesetId)
	}
	for outputId := range project.GlobalProject.Outputs {
		samplerNames = append(samplerNames, "output."+outputId)
	}
	common.GlobalMu.RUnlock()

	return samplerNames
}

func prioritizeDataSources(dataSources []string, rulePurpose string) []string {
	prioritized := make([]string, 0)
	regular := make([]string, 0)
	purposeLower := strings.ToLower(rulePurpose)

	for _, source := range dataSources {
		sourceLower := strings.ToLower(source)
		if (strings.Contains(purposeLower, "network") && strings.Contains(sourceLower, "net")) ||
			(strings.Contains(purposeLower, "security") && strings.Contains(sourceLower, "sec")) ||
			(strings.Contains(purposeLower, "api") && strings.Contains(sourceLower, "api")) {
			prioritized = append(prioritized, source)
		} else {
			regular = append(regular, source)
		}
	}

	return append(prioritized, regular...)
}

func getSourcePriority(samplerName string, prioritizedSources []string) int {
	for i, source := range prioritizedSources {
		if strings.Contains(samplerName, source) {
			return len(prioritizedSources) - i
		}
	}
	return 0
}

func meetsQualityThreshold(data interface{}, threshold float64) bool {
	if dataMap, ok := data.(map[string]interface{}); ok {
		return len(dataMap) >= int(threshold*10)
	}
	return false
}

func hasRequiredFields(data interface{}, requiredFields []string) bool {
	if dataMap, ok := data.(map[string]interface{}); ok {
		for _, field := range requiredFields {
			if _, exists := dataMap[field]; !exists {
				return false
			}
		}
	}
	return true
}

func analyzeDataQuality(sampleData map[string]interface{}, requiredFields []string) DataQualityAnalysis {
	analysis := DataQualityAnalysis{
		QualityIssues:     make([]string, 0),
		FieldDistribution: make(map[string]int),
	}

	totalSamples := 0
	fieldCounts := make(map[string]int)
	latestTimestamp := time.Time{}

	// Analyze samples
	for _, flowData := range sampleData {
		if samples, ok := flowData.([]interface{}); ok {
			for _, sampleIntf := range samples {
				if sample, ok := sampleIntf.(map[string]interface{}); ok {
					totalSamples++

					if timestampStr, ok := sample["timestamp"].(string); ok {
						if ts, err := time.Parse(time.RFC3339, timestampStr); err == nil && ts.After(latestTimestamp) {
							latestTimestamp = ts
						}
					}

					if data, ok := sample["data"].(map[string]interface{}); ok {
						analyzeDataFields(data, "", fieldCounts)
					}
				}
			}
		}
	}

	analysis.VolumeEstimate = int64(totalSamples * 24)
	if !latestTimestamp.IsZero() {
		analysis.DataFreshness = time.Since(latestTimestamp).String()
	}

	// Calculate field coverage
	if len(requiredFields) > 0 {
		foundFields := 0
		for _, field := range requiredFields {
			if fieldCounts[field] > 0 {
				foundFields++
			}
		}
		analysis.FieldCoverage = float64(foundFields) / float64(len(requiredFields))
	} else {
		analysis.FieldCoverage = 1.0
	}

	analysis.FieldDistribution = fieldCounts

	// Calculate overall score
	freshnessScore := calculateFreshnessScore(latestTimestamp)
	volumeScore := calculateVolumeScore(totalSamples)
	analysis.OverallScore = (analysis.FieldCoverage*0.4 + freshnessScore*0.3 + volumeScore*0.3)

	// Identify issues
	if analysis.FieldCoverage < 0.8 {
		analysis.QualityIssues = append(analysis.QualityIssues, "Missing required fields")
	}
	if totalSamples < 10 {
		analysis.QualityIssues = append(analysis.QualityIssues, "Insufficient sample size")
	}
	if time.Since(latestTimestamp) > 24*time.Hour {
		analysis.QualityIssues = append(analysis.QualityIssues, "Data may be stale")
	}

	return analysis
}

func analyzeFieldUsage(sampleData map[string]interface{}, rulePurpose string) FieldUsageAnalysis {
	analysis := FieldUsageAnalysis{
		AvailableFields:    make([]string, 0),
		RecommendedFields:  make([]string, 0),
		FieldTypes:         make(map[string]string),
		ValueDistributions: make(map[string]FieldValueStats),
		PerformanceImpact:  make(map[string]string),
	}

	fieldStats := make(map[string]map[interface{}]int)
	fieldTypes := make(map[string]map[string]int)

	// Collect field statistics
	for _, flowData := range sampleData {
		if samples, ok := flowData.([]interface{}); ok {
			for _, sampleIntf := range samples {
				if sample, ok := sampleIntf.(map[string]interface{}); ok {
					if data, ok := sample["data"].(map[string]interface{}); ok {
						collectFieldStats(data, "", fieldStats, fieldTypes)
					}
				}
			}
		}
	}

	// Process statistics
	for fieldName := range fieldStats {
		analysis.AvailableFields = append(analysis.AvailableFields, fieldName)

		if typeMap, exists := fieldTypes[fieldName]; exists {
			analysis.FieldTypes[fieldName] = getMostCommonType(typeMap)
		}

		stats := FieldValueStats{
			UniqueValues: len(fieldStats[fieldName]),
			DataType:     analysis.FieldTypes[fieldName],
		}
		analysis.ValueDistributions[fieldName] = stats
		analysis.PerformanceImpact[fieldName] = assessFieldPerformanceImpact(fieldName, stats)
	}

	sort.Strings(analysis.AvailableFields)
	analysis.RecommendedFields = recommendFieldsForPurpose(analysis.AvailableFields, rulePurpose)

	return analysis
}

func generateDataRecommendations(quality DataQualityAnalysis, fieldAnalysis FieldUsageAnalysis, context ProjectContextInfo) []string {
	recommendations := make([]string, 0)

	if quality.OverallScore < 0.6 {
		recommendations = append(recommendations, "Consider improving data quality before creating rules")
	}

	if quality.FieldCoverage < 0.8 {
		recommendations = append(recommendations, "Some required fields are missing - verify data sources")
	}

	if len(fieldAnalysis.RecommendedFields) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Recommended fields: %s",
				strings.Join(fieldAnalysis.RecommendedFields[:min(3, len(fieldAnalysis.RecommendedFields))], ", ")))
	}

	if len(context.SuggestedProjects) > 0 {
		projectNames := make([]string, len(context.SuggestedProjects))
		for i, proj := range context.SuggestedProjects {
			projectNames[i] = proj.ID
		}
		recommendations = append(recommendations,
			fmt.Sprintf("Consider similar projects: %s", strings.Join(projectNames, ", ")))
	}

	return recommendations
}

func analyzeDataFields(data map[string]interface{}, prefix string, fieldCounts map[string]int) {
	for key, value := range data {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}
		fieldCounts[fieldName]++

		if nestedMap, ok := value.(map[string]interface{}); ok {
			analyzeDataFields(nestedMap, fieldName, fieldCounts)
		}
	}
}

func calculateFreshnessScore(timestamp time.Time) float64 {
	if timestamp.IsZero() {
		return 0.5
	}

	age := time.Since(timestamp)
	if age < time.Hour {
		return 1.0
	} else if age < 24*time.Hour {
		return 0.8
	} else if age < 7*24*time.Hour {
		return 0.6
	}
	return 0.3
}

func calculateVolumeScore(sampleCount int) float64 {
	if sampleCount >= 100 {
		return 1.0
	} else if sampleCount >= 50 {
		return 0.8
	} else if sampleCount >= 10 {
		return 0.6
	}
	return 0.3
}

func collectFieldStats(data map[string]interface{}, prefix string,
	fieldStats map[string]map[interface{}]int, fieldTypes map[string]map[string]int) {

	for key, value := range data {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}

		if fieldStats[fieldName] == nil {
			fieldStats[fieldName] = make(map[interface{}]int)
		}
		if fieldTypes[fieldName] == nil {
			fieldTypes[fieldName] = make(map[string]int)
		}

		fieldStats[fieldName][value]++
		fieldTypes[fieldName][fmt.Sprintf("%T", value)]++

		if nestedMap, ok := value.(map[string]interface{}); ok {
			collectFieldStats(nestedMap, fieldName, fieldStats, fieldTypes)
		}
	}
}

func getMostCommonType(typeMap map[string]int) string {
	maxCount := 0
	mostCommon := "unknown"

	for typeName, count := range typeMap {
		if count > maxCount {
			maxCount = count
			mostCommon = typeName
		}
	}

	return mostCommon
}

func assessFieldPerformanceImpact(fieldName string, stats FieldValueStats) string {
	if stats.UniqueValues > 1000 {
		return "high"
	} else if stats.UniqueValues > 100 {
		return "medium"
	}
	return "low"
}

func recommendFieldsForPurpose(availableFields []string, rulePurpose string) []string {
	recommendations := make([]string, 0)
	purposeLower := strings.ToLower(rulePurpose)

	for _, field := range availableFields {
		fieldLower := strings.ToLower(field)

		if strings.Contains(purposeLower, "network") {
			if strings.Contains(fieldLower, "ip") || strings.Contains(fieldLower, "port") ||
				strings.Contains(fieldLower, "protocol") {
				recommendations = append(recommendations, field)
			}
		}

		if strings.Contains(purposeLower, "security") {
			if strings.Contains(fieldLower, "security") || strings.Contains(fieldLower, "threat") {
				recommendations = append(recommendations, field)
			}
		}

		if strings.Contains(purposeLower, "error") {
			if strings.Contains(fieldLower, "error") || strings.Contains(fieldLower, "status") {
				recommendations = append(recommendations, field)
			}
		}
	}

	if len(recommendations) > 5 {
		recommendations = recommendations[:5]
	}

	return recommendations
}

// Extract target component from request context
func extractTargetComponent(req IntelligentSampleDataRequest, context ProjectContextInfo) string {
	// For rule creation, the target is typically a ruleset
	// Check field requirements for hints
	for _, field := range req.FieldRequirements {
		if strings.ToLower(field) == "ruleset" || strings.ToLower(field) == "rule" {
			// Look for ruleset in target projects
			for range context.TargetProjects {
				// This is likely the project containing the target ruleset
				// In the current case, we're looking for "test" ruleset
				return "ruleset.test" // Based on the user's scenario
			}
		}
	}

	// Default assumption for rule creation scenarios
	return "ruleset.test"
}

// Get sample data from a specific component
func getSampleDataFromComponent(componentName string) []interface{} {
	samples := make([]interface{}, 0)

	sampler := common.GetSampler(componentName)
	if sampler != nil {
		samplerData := sampler.GetSamples()

		for projectNodeSequence, sampleDataList := range samplerData {
			for _, sample := range sampleDataList {
				convertedSample := map[string]interface{}{
					"data":                  sample.Data,
					"timestamp":             sample.Timestamp.Format(time.RFC3339),
					"project_node_sequence": projectNodeSequence,
					"source":                componentName,
				}
				samples = append(samples, convertedSample)
			}
		}
	}

	return samples
}

// Find projects that use a specific component
func findProjectsUsingComponent(componentName string) []string {
	projects := make([]string, 0)

	// Extract component type and ID
	parts := strings.Split(componentName, ".")
	if len(parts) != 2 {
		return projects
	}

	componentType := parts[0]
	componentID := parts[1]

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for projectID, proj := range project.GlobalProject.Projects {
		switch componentType {
		case "ruleset":
			if _, exists := proj.Rulesets[componentID]; exists {
				projects = append(projects, projectID)
			}
		case "input":
			if _, exists := proj.Inputs[componentID]; exists {
				projects = append(projects, projectID)
			}
		case "output":
			if _, exists := proj.Outputs[componentID]; exists {
				projects = append(projects, projectID)
			}
		}
	}

	return projects
}

// Find upstream input components for a target component in a specific project
func findUpstreamInputs(projectID string, targetComponent string) []string {
	inputs := make([]string, 0)

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	proj, exists := project.GlobalProject.Projects[projectID]
	if !exists {
		return inputs
	}

	// Parse the project content to understand data flow
	// Format: INPUT.id -> RULESET.id -> OUTPUT.id
	content := ""
	if proj.Config != nil {
		content = proj.Config.Content
	}
	if content == "" {
		return inputs
	}

	// Extract component type and ID from target
	parts := strings.Split(targetComponent, ".")
	if len(parts) != 2 {
		return inputs
	}

	targetType := strings.ToUpper(parts[0])
	targetID := parts[1]

	// Find flows that lead to the target component
	flowParts := strings.Split(content, "->")
	for i, part := range flowParts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, fmt.Sprintf("%s.%s", targetType, targetID)) {
			// Found target component, look at previous part for upstream
			if i > 0 {
				prevPart := strings.TrimSpace(flowParts[i-1])
				// Extract input components from previous part
				if strings.Contains(prevPart, "INPUT.") {
					inputMatches := extractComponentIDs(prevPart, "INPUT")
					inputs = append(inputs, inputMatches...)
				}
			}
		}
	}

	return inputs
}

// Extract component IDs of a specific type from flow content
func extractComponentIDs(content string, componentType string) []string {
	ids := make([]string, 0)

	// Split by commas for multiple components
	parts := strings.Split(content, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, componentType+".") {
			id := strings.TrimPrefix(part, componentType+".")
			ids = append(ids, id)
		}
	}

	return ids
}

// Get fallback sample data from any available source
func getFallbackSampleData() []interface{} {
	samples := make([]interface{}, 0)

	// Try input components first as they're most likely to have real data
	samplerNames := getAllSamplerNames()
	for _, samplerName := range samplerNames {
		if strings.HasPrefix(samplerName, "input.") {
			componentSamples := getSampleDataFromComponent(samplerName)
			if len(componentSamples) > 0 {
				return componentSamples
			}
		}
	}

	return samples
}

// Extract target ruleset from field requirements or rule purpose
func extractTargetRulesetFromRequest(req IntelligentSampleDataRequest) string {
	// Check field requirements for explicit ruleset
	for _, field := range req.FieldRequirements {
		if field != "" && !strings.Contains(field, " ") {
			// Assume it's a ruleset ID
			return field
		}
	}

	// Try to extract from rule purpose (e.g., "检测exe为msf的数据" -> might be for "test" ruleset)
	// For now, default to common patterns or let user specify
	return ""
}

// Get sample data for a specific ruleset
// This implements the correct logic:
// 1. Find projects using this ruleset
// 2. Get upstream input components from those projects
// 3. Return sample data from those inputs to understand data structure
func getSampleDataForRuleset(rulesetID string) (map[string]interface{}, string, error) {
	logger.Info("Getting sample data for ruleset", "rulesetID", rulesetID)

	// Step 1: Find projects that use this ruleset
	usingProjects := findProjectsUsingRuleset(rulesetID)
	if len(usingProjects) == 0 {
		logger.Info("No projects found using ruleset", "rulesetID", rulesetID)
		return map[string]interface{}{}, "", fmt.Errorf("no projects use ruleset %s", rulesetID)
	}

	logger.Info("Found projects using ruleset", "rulesetID", rulesetID, "projects", usingProjects)

	// Step 2: For each project, find upstream input components
	for _, projectID := range usingProjects {
		upstreamInputs := findUpstreamInputsForRuleset(projectID, rulesetID)
		logger.Info("Found upstream inputs", "project", projectID, "ruleset", rulesetID, "inputs", upstreamInputs)

		// Step 3: Try to get sample data from upstream inputs
		for _, inputID := range upstreamInputs {
			inputSamples := getSampleDataFromComponent("input." + inputID)
			if len(inputSamples) > 0 {
				logger.Info("Found sample data from input", "input", inputID, "samples", len(inputSamples))

				// Format the result
				result := map[string]interface{}{
					fmt.Sprintf("%s.%s->ruleset.%s", projectID, inputID, rulesetID): inputSamples,
				}
				dataSource := fmt.Sprintf("input.%s (via project.%s)", inputID, projectID)
				return result, dataSource, nil
			}
		}
	}

	// Step 4: If no upstream input data found, return empty but with clear message
	logger.Info("No sample data found in upstream inputs", "rulesetID", rulesetID)
	return map[string]interface{}{}, "", fmt.Errorf("no sample data available from upstream inputs for ruleset %s", rulesetID)
}

// Find projects that use a specific ruleset
func findProjectsUsingRuleset(rulesetID string) []string {
	projects := make([]string, 0)

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for projectID, proj := range project.GlobalProject.Projects {
		if _, exists := proj.Rulesets[rulesetID]; exists {
			projects = append(projects, projectID)
		}
	}

	return projects
}

// Get sample data for a specific input component
func getSampleDataForInput(inputID string) ([]interface{}, string, error) {
	logger.Info("Getting sample data for input", "inputID", inputID)

	// Direct sample data from the input component
	inputSamples := getSampleDataFromComponent("input." + inputID)
	if len(inputSamples) > 0 {
		logger.Info("Found sample data from input", "input", inputID, "samples", len(inputSamples))
		dataSource := fmt.Sprintf("input.%s (direct source)", inputID)
		return inputSamples, dataSource, nil
	}

	// If no direct data, return empty but with clear message
	logger.Info("No sample data found for input", "inputID", inputID)
	return []interface{}{}, "", fmt.Errorf("no sample data available for input %s", inputID)
}

// Get sample data for a specific output component
func getSampleDataForOutput(outputID string) ([]interface{}, string, error) {
	logger.Info("Getting sample data for output", "outputID", outputID)

	// Find projects that use this output and get data from their upstream components
	usingProjects := findProjectsUsingOutput(outputID)
	if len(usingProjects) == 0 {
		logger.Info("No projects found using output", "outputID", outputID)
		return []interface{}{}, "", fmt.Errorf("no projects use output %s", outputID)
	}

	logger.Info("Found projects using output", "outputID", outputID, "projects", usingProjects)

	// For each project, find what flows to this output
	for _, projectID := range usingProjects {
		upstreamComponents := findUpstreamComponentsForOutput(projectID, outputID)
		logger.Info("Found upstream components", "project", projectID, "output", outputID, "components", upstreamComponents)

		// Try to get sample data from upstream components
		for _, component := range upstreamComponents {
			samples := getSampleDataFromComponent(component)
			if len(samples) > 0 {
				logger.Info("Found sample data from component", "component", component, "samples", len(samples))
				dataSource := fmt.Sprintf("%s (via project.%s)", component, projectID)
				return samples, dataSource, nil
			}
		}
	}

	// If no upstream data found, return empty but with clear message
	logger.Info("No sample data found in upstream components", "outputID", outputID)
	return []interface{}{}, "", fmt.Errorf("no sample data available from upstream components for output %s", outputID)
}

// Find projects that use a specific output
func findProjectsUsingOutput(outputID string) []string {
	projects := make([]string, 0)

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for projectID, proj := range project.GlobalProject.Projects {
		if _, exists := proj.Outputs[outputID]; exists {
			projects = append(projects, projectID)
		}
	}

	return projects
}

// Find upstream components that flow to a specific output in a project
func findUpstreamComponentsForOutput(projectID string, outputID string) []string {
	components := make([]string, 0)

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	proj, exists := project.GlobalProject.Projects[projectID]
	if !exists {
		return components
	}

	// Parse the project content to understand data flow
	// Format: INPUT.id -> RULESET.id -> OUTPUT.id
	content := ""
	if proj.Config != nil {
		content = proj.Config.Content
	}
	if content == "" {
		return components
	}

	logger.Info("Analyzing project content for output", "project", projectID, "content", content)

	// Find flows that lead to the target output
	// Example: "INPUT.kafka_logs -> RULESET.test -> OUTPUT.es_alerts"
	flowParts := strings.Split(content, "->")
	for i, part := range flowParts {
		part = strings.TrimSpace(part)
		outputPattern := fmt.Sprintf("OUTPUT.%s", outputID)

		if strings.Contains(part, outputPattern) {
			logger.Info("Found output in flow", "part", part, "output", outputID)
			// Found target output, look at all previous parts for upstream components
			for j := i - 1; j >= 0; j-- {
				prevPart := strings.TrimSpace(flowParts[j])
				logger.Info("Checking previous part for components", "prevPart", prevPart)

				// Extract all components from previous parts
				if strings.Contains(prevPart, "INPUT.") {
					inputMatches := extractComponentIDs(prevPart, "INPUT")
					for _, inputID := range inputMatches {
						components = append(components, "input."+inputID)
					}
				}
				if strings.Contains(prevPart, "RULESET.") {
					rulesetMatches := extractComponentIDs(prevPart, "RULESET")
					for _, rulesetID := range rulesetMatches {
						components = append(components, "ruleset."+rulesetID)
					}
				}
			}
		}
	}

	return components
}

// Get sample data for a specific project by analyzing its data flow
func getSampleDataForProject(projectID string) (map[string]interface{}, string, error) {
	logger.Info("Getting sample data for project", "projectID", projectID)

	common.GlobalMu.RLock()
	proj, exists := project.GlobalProject.Projects[projectID]
	common.GlobalMu.RUnlock()

	if !exists {
		return map[string]interface{}{}, "", fmt.Errorf("project %s not found", projectID)
	}

	// Parse the project content to understand data flow
	content := ""
	if proj.Config != nil {
		content = proj.Config.Content
	}
	if content == "" {
		return map[string]interface{}{}, "", fmt.Errorf("project %s has no data flow configuration", projectID)
	}

	logger.Info("Analyzing project data flow", "project", projectID, "content", content)

	result := make(map[string]interface{})
	dataSources := make([]string, 0)

	// Extract all input components from the project flow
	if strings.Contains(content, "INPUT.") {
		inputIDs := extractComponentIDs(content, "INPUT")
		for _, inputID := range inputIDs {
			inputSamples := getSampleDataFromComponent("input." + inputID)
			if len(inputSamples) > 0 {
				flowKey := fmt.Sprintf("input.%s->project.%s", inputID, projectID)
				result[flowKey] = inputSamples
				dataSources = append(dataSources, "input."+inputID)
			}
		}
	}

	if len(result) > 0 {
		dataSource := fmt.Sprintf("project.%s inputs: %s", projectID, strings.Join(dataSources, ", "))
		return result, dataSource, nil
	}

	return map[string]interface{}{}, "", fmt.Errorf("no sample data available for project %s", projectID)
}

// Find upstream input components that flow to a specific ruleset in a project
func findUpstreamInputsForRuleset(projectID string, rulesetID string) []string {
	inputs := make([]string, 0)

	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	proj, exists := project.GlobalProject.Projects[projectID]
	if !exists {
		return inputs
	}

	// Parse the project content to understand data flow
	// Format: INPUT.id -> RULESET.id -> OUTPUT.id
	content := ""
	if proj.Config != nil {
		content = proj.Config.Content
	}
	if content == "" {
		return inputs
	}

	logger.Info("Analyzing project content", "project", projectID, "content", content)

	// Find flows that lead to the target ruleset
	// Example: "INPUT.kafka_logs -> RULESET.test -> OUTPUT.es_alerts"
	flowParts := strings.Split(content, "->")
	for i, part := range flowParts {
		part = strings.TrimSpace(part)
		rulesetPattern := fmt.Sprintf("RULESET.%s", rulesetID)

		if strings.Contains(part, rulesetPattern) {
			logger.Info("Found ruleset in flow", "part", part, "ruleset", rulesetID)
			// Found target ruleset, look at previous part for upstream inputs
			if i > 0 {
				prevPart := strings.TrimSpace(flowParts[i-1])
				logger.Info("Checking previous part for inputs", "prevPart", prevPart)

				// Extract input components from previous part
				if strings.Contains(prevPart, "INPUT.") {
					inputMatches := extractComponentIDs(prevPart, "INPUT")
					inputs = append(inputs, inputMatches...)
					logger.Info("Extracted input IDs", "inputs", inputMatches)
				}
			}
		}
	}

	return inputs
}
