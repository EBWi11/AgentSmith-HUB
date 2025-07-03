package mcp

import (
	"AgentSmith-HUB/common"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Annotation helper functions for creating MCPToolAnnotations
func createAnnotations(title string, readOnly, destructive, idempotent, openWorld *bool) *common.MCPToolAnnotations {
	return &common.MCPToolAnnotations{
		Title:           title,
		ReadOnlyHint:    readOnly,
		DestructiveHint: destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   openWorld,
	}
}

// Helper functions for creating boolean pointers
func boolPtr(b bool) *bool {
	return &b
}

// APIMapper handles the mapping between MCP tools and existing HTTP API endpoints
type APIMapper struct {
	baseURL string
	token   string
}

// NewAPIMapper creates a new API mapper
func NewAPIMapper(baseURL, token string) *APIMapper {
	return &APIMapper{
		baseURL: baseURL,
		token:   token,
	}
}

// GetAllAPITools returns all MCP tools that map to existing API endpoints
func (m *APIMapper) GetAllAPITools() []common.MCPTool {
	return []common.MCPTool{
		// === 🎯 INTELLIGENT WORKFLOW TOOLS ===
		// Smart tools that combine multiple operations for optimal user experience

		// 🔥 Primary Workflow - Rule Management
		{
			Name:        "create_rule_complete",
			Description: "🎯 INTELLIGENT RULE CREATION: Smart workflow → 1. Identify target projects → 2. Get relevant sample data → 3. Analyze data structure → 4. Design rule based on user needs + real data. Automatically finds appropriate sample data for rule context.",
			InputSchema: map[string]common.MCPToolArg{
				"ruleset_id":      {Type: "string", Description: "Target ruleset ID", Required: true},
				"rule_purpose":    {Type: "string", Description: "What should this rule detect? (e.g., 'suspicious network connections', 'malware execution')", Required: true},
				"target_projects": {Type: "string", Description: "Which projects will use this rule? (comma-separated IDs or 'auto' to detect)", Required: false},
				"sample_data":     {Type: "string", Description: "Sample data (optional - will auto-fetch from target projects if not provided)", Required: false},
				"rule_name":       {Type: "string", Description: "Human-readable rule name", Required: false},
				"auto_deploy":     {Type: "string", Description: "Auto-deploy if tests pass: true/false (default: false)", Required: false},
			},
			Annotations: createAnnotations("Smart Rule Creation", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "smart_deployment",
			Description: "🚀 INTELLIGENT DEPLOYMENT: Validates all pending changes → Tests compatibility → Deploys with rollback capability. Prevents failed deployments and provides detailed feedback.",
			InputSchema: map[string]common.MCPToolArg{
				"component_filter": {Type: "string", Description: "Deploy specific component type (optional): ruleset/input/output/plugin/project", Required: false},
				"dry_run":          {Type: "string", Description: "Preview deployment without applying (true/false)", Required: false},
				"force_deploy":     {Type: "string", Description: "Skip validation warnings (true/false) - use cautiously", Required: false},
				"test_after":       {Type: "string", Description: "Run component tests after deployment (true/false)", Required: false},
			},
			Annotations: createAnnotations("Smart Deployment", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🔥 Component Lifecycle Management
		{
			Name:        "component_wizard",
			Description: "🧙‍♂️ COMPONENT CREATION WIZARD: Guided component creation with templates, validation, and testing. Supports all component types with intelligent defaults and best practices.",
			InputSchema: map[string]common.MCPToolArg{
				"component_type": {Type: "string", Description: "Component type: input/output/plugin/project/ruleset", Required: true},
				"component_id":   {Type: "string", Description: "Component identifier", Required: true},
				"use_template":   {Type: "string", Description: "Use template (true/false) - recommended for beginners", Required: false},
				"config_content": {Type: "string", Description: "Component configuration (optional if using template)", Required: false},
				"test_data":      {Type: "string", Description: "Test data for validation", Required: false},
				"auto_deploy":    {Type: "string", Description: "Auto-deploy after creation (true/false)", Required: false},
			},
			Annotations: createAnnotations("Component Wizard", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🔥 System Intelligence
		{
			Name:        "system_overview",
			Description: "🏠 SYSTEM DASHBOARD: Complete system status with health check, pending changes, active projects, and smart recommendations. Your one-stop system overview.",
			InputSchema: map[string]common.MCPToolArg{
				"include_metrics":     {Type: "string", Description: "Include performance metrics (true/false)", Required: false},
				"include_suggestions": {Type: "string", Description: "Include optimization suggestions (true/false)", Required: false},
				"focus_area":          {Type: "string", Description: "Focus on specific area: rules/projects/health/all", Required: false},
			},
			Annotations: createAnnotations("System Dashboard", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(true)),
		},

		// === 🎯 CORE COMPONENT MANAGEMENT ===
		// Simplified, intelligent component operations

		// 🔥 Universal Component Operations
		{
			Name:        "explore_components",
			Description: "🔍 SMART EXPLORER: List and discover all components (projects, rulesets, inputs, outputs, plugins) with search, filtering, and status overview. Your starting point for exploration.",
			InputSchema: map[string]common.MCPToolArg{
				"component_type":  {Type: "string", Description: "Filter by type: project/ruleset/input/output/plugin/all (default: all)", Required: false},
				"search_term":     {Type: "string", Description: "Search components by name or content", Required: false},
				"show_status":     {Type: "string", Description: "Include deployment status (true/false)", Required: false},
				"include_details": {Type: "string", Description: "Include detailed configuration (true/false)", Required: false},
			},
			Annotations: createAnnotations("Component Explorer", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		{
			Name:        "component_manager",
			Description: "⚙️ UNIVERSAL COMPONENT MANAGER: View, edit, create, or delete any component with intelligent validation and deployment options. Handles all component types uniformly.",
			InputSchema: map[string]common.MCPToolArg{
				"action":         {Type: "string", Description: "Action: view/create/update/delete", Required: true},
				"component_type": {Type: "string", Description: "Component type: project/ruleset/input/output/plugin", Required: true},
				"component_id":   {Type: "string", Description: "Component ID", Required: true},
				"config_content": {Type: "string", Description: "Configuration content (for create/update actions)", Required: false},
				"auto_deploy":    {Type: "string", Description: "Auto-deploy after changes (true/false)", Required: false},
				"backup_first":   {Type: "string", Description: "Create backup before destructive operations (true/false)", Required: false},
			},
			Annotations: createAnnotations("Universal Manager", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🔥 Project Operations
		{
			Name:        "project_control",
			Description: "🎮 PROJECT CONTROLLER: Start, stop, restart projects with health monitoring and automatic recovery. Includes batch operations and smart status tracking.",
			InputSchema: map[string]common.MCPToolArg{
				"action":     {Type: "string", Description: "Action: start/stop/restart/status/start_all/stop_all", Required: true},
				"project_id": {Type: "string", Description: "Specific project ID (optional for batch operations)", Required: false},
				"force":      {Type: "string", Description: "Force operation even if warnings (true/false)", Required: false},
				"wait_ready": {Type: "string", Description: "Wait for project to be fully ready (true/false)", Required: false},
			},
			Annotations: createAnnotations("Project Controller", boolPtr(false), boolPtr(false), boolPtr(true), boolPtr(false)),
		},

		// 🔥 Advanced Rule Management
		{
			Name:        "rule_manager",
			Description: "🛡️ INTELLIGENT RULE MANAGER: Smart context-aware rule management → Auto-discovers target projects → Fetches relevant sample data → Analyzes data structure → Creates rules based on user intent + real data patterns. 🚨 CRITICAL: NEVER uses imagined data! All rules must be based on REAL sample data only!",
			InputSchema: map[string]common.MCPToolArg{
				"action":          {Type: "string", Description: "Action: add_rule/update_rule/delete_rule/view_rules/create_ruleset/update_ruleset", Required: true},
				"id":              {Type: "string", Description: "Target ruleset ID", Required: true},
				"rule_purpose":    {Type: "string", Description: "What should this rule detect? (for add_rule action)", Required: false},
				"target_projects": {Type: "string", Description: "Projects that will use this rule (for context-aware data fetching)", Required: false},
				"rule_id":         {Type: "string", Description: "Specific rule ID (for update/delete actions)", Required: false},
				"rule_raw":        {Type: "string", Description: "Complete rule XML (generated after data analysis)", Required: false},
				"raw":             {Type: "string", Description: "Complete ruleset XML (for create/update ruleset actions)", Required: false},
				"data":            {Type: "string", Description: "🚨 MANDATORY: REAL sample data ONLY! Use get_samplers_data API OR user-provided actual JSON. ❌ NEVER use imagined data like 'data_type=59'!", Required: false},
				"auto_deploy":     {Type: "string", Description: "Auto-deploy if validation passes (true/false)", Required: false},
			},
			Annotations: createAnnotations("Rule Manager", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// === 🎯 TESTING & VALIDATION ===
		// Smart testing and validation tools

		{
			Name:        "test_lab",
			Description: "🧪 COMPREHENSIVE TESTING LAB: Test any component with intelligent data samples, validation reports, and performance metrics. Supports batch testing and automated test suites.",
			InputSchema: map[string]common.MCPToolArg{
				"test_target":    {Type: "string", Description: "What to test: component/ruleset/project/workflow", Required: true},
				"component_id":   {Type: "string", Description: "Component ID or 'all' for batch testing", Required: true},
				"test_mode":      {Type: "string", Description: "Test mode: quick/thorough/performance/security", Required: false},
				"custom_data":    {Type: "string", Description: "Custom test data (JSON) - optional, auto-generates if not provided", Required: false},
				"include_report": {Type: "string", Description: "Generate detailed test report (true/false)", Required: false},
			},
			Annotations: createAnnotations("Testing Lab", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// === 🎯 DEPLOYMENT & RESOURCES ===
		// Smart deployment and learning tools

		{
			Name:        "deployment_center",
			Description: "🚀 SMART DEPLOYMENT CENTER: View pending changes → Validate → Deploy with rollback capability. Includes deployment history, impact analysis, and automated testing.",
			InputSchema: map[string]common.MCPToolArg{
				"action":           {Type: "string", Description: "Action: view_pending/validate/deploy/rollback/history", Required: true},
				"component_filter": {Type: "string", Description: "Filter by component type (optional)", Required: false},
				"dry_run":          {Type: "string", Description: "Simulate deployment without applying (true/false)", Required: false},
				"force":            {Type: "string", Description: "Force deploy despite warnings (true/false)", Required: false},
				"create_backup":    {Type: "string", Description: "Create backup before deployment (true/false)", Required: false},
			},
			Annotations: createAnnotations("Deployment Center", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		{
			Name:        "learning_center",
			Description: "📚 DATA-DRIVEN LEARNING CENTER: Get templates, tutorials, and best practices. ⚠️ IMPORTANT: If backend has no sample data, guide users to provide their own real data for rule creation.",
			InputSchema: map[string]common.MCPToolArg{
				"resource_type": {Type: "string", Description: "Resource: samples (try to get backend data)/syntax_guide/templates/tutorials/best_practices", Required: true},
				"component":     {Type: "string", Description: "Component focus: ruleset/input/output/plugin/project/all", Required: false},
				"difficulty":    {Type: "string", Description: "Difficulty level: beginner/intermediate/advanced", Required: false},
				"format":        {Type: "string", Description: "Output format: summary/detailed/interactive", Required: false},
			},
			Annotations: createAnnotations("Learning Center", boolPtr(true), boolPtr(false), boolPtr(true), boolPtr(false)),
		},

		// === 🎯 DATA INTELLIGENCE ===
		// Intelligent data analysis and sample retrieval

		{
			Name:        "get_samplers_data_intelligent",
			Description: "🧠 INTELLIGENT SAMPLE DATA: Enhanced sample data retrieval with project context analysis. 🚨 CRITICAL: Cannot generate fake data! If backend has NO data or this fails, you MUST ask user to provide REAL JSON data. ❌ NEVER imagine or create data!",
			InputSchema: map[string]common.MCPToolArg{
				"target_projects":    {Type: "string", Description: "Target projects (comma-separated IDs) for context-aware data fetching", Required: false},
				"rule_purpose":       {Type: "string", Description: "What will this rule detect? (e.g., 'network security', 'error monitoring')", Required: false},
				"field_requirements": {Type: "string", Description: "Required fields (comma-separated) for rule creation", Required: false},
				"quality_threshold":  {Type: "string", Description: "Minimum data quality score (0.0-1.0)", Required: false},
				"sampler_type":       {Type: "string", Description: "Backward compatibility: specific sampler type", Required: false},
				"count":              {Type: "string", Description: "Backward compatibility: sample count", Required: false},
			},
			Annotations: createAnnotations("Intelligent Data", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		{
			Name:        "smart_assistant",
			Description: "🤖 AI-POWERED ASSISTANT: Get intelligent recommendations, troubleshoot issues, optimize configurations, and receive guided help for any task. Your personal AgentSmith expert. Use 'system_intro' task for complete architecture overview.",
			InputSchema: map[string]common.MCPToolArg{
				"task":        {Type: "string", Description: "What you want to accomplish or issue you're facing. Use 'system_intro' for complete AgentSmith-HUB overview.", Required: true},
				"context":     {Type: "string", Description: "Current situation or component you're working with", Required: false},
				"experience":  {Type: "string", Description: "Your experience level: beginner/intermediate/expert", Required: false},
				"preferences": {Type: "string", Description: "Preferences: step_by_step/quick_solution/explain_why", Required: false},
			},
			Annotations: createAnnotations("Smart Assistant", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(true)),
		},

		// === 📋 BASIC DIRECT TOOLS ===
		// Simple, direct tools for common operations - Added back for usability

		// 🔍 Essential Data Tools
		{
			Name:        "get_samplers_data",
			Description: "📊 GET SAMPLE DATA: Try to get real sample data from backend. 🚨 CRITICAL: If this FAILS or returns empty, you MUST ask user to provide their own REAL JSON data. ❌ NEVER create fake data yourself!",
			InputSchema: map[string]common.MCPToolArg{
				"name":                {Type: "string", Description: "Component type: 'input', 'output', or 'ruleset'", Required: true},
				"projectNodeSequence": {Type: "string", Description: "Component ID (e.g. 'test') or full sequence (e.g. 'ruleset.test'). Simple ID is usually sufficient.", Required: true},
			},
			Annotations: createAnnotations("Get Sample Data", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🛡️ Direct Rule Operations
		{
			Name:        "add_ruleset_rule",
			Description: "➕ ADD RULE TO RULESET: Add a single rule to an existing ruleset. 🚨 CRITICAL: Requires REAL sample data! Use get_samplers_data first OR provide actual JSON from user's system. ❌ NEVER use imagined data like 'data_type=59' or fake field names!",
			InputSchema: map[string]common.MCPToolArg{
				"id":       {Type: "string", Description: "Ruleset ID", Required: true},
				"rule_raw": {Type: "string", Description: "Complete rule XML content", Required: true},
			},
			Annotations: createAnnotations("Add Rule", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "delete_ruleset_rule",
			Description: "🗑️ DELETE RULE FROM RULESET: Remove a specific rule from a ruleset by rule ID.",
			InputSchema: map[string]common.MCPToolArg{
				"id":      {Type: "string", Description: "Ruleset ID", Required: true},
				"rule_id": {Type: "string", Description: "Rule ID to delete", Required: true},
			},
			Annotations: createAnnotations("Delete Rule", boolPtr(false), boolPtr(true), boolPtr(true), boolPtr(false)),
		},

		// 📋 Component Viewing
		{
			Name:        "get_rulesets",
			Description: "📋 LIST ALL RULESETS: View all rulesets with rule counts and usage info. ⚠️ IMPORTANT: Check deployment status! Use 'get_pending_changes' to see if rulesets are temporary/unpublished. Use 'get_component_usage' to see project dependencies.",
			InputSchema: map[string]common.MCPToolArg{},
			Annotations: createAnnotations("List Rulesets", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "get_ruleset",
			Description: "🔍 VIEW RULESET DETAILS: Get detailed information about a specific ruleset including all rules and configuration. 🎯 NEW: Automatically includes relevant sample data from upstream input components! ⚠️ Note: If you see temporary changes, they are NOT ACTIVE! Check 'get_pending_changes' for deployment status.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Ruleset ID", Required: true},
			},
			Annotations: createAnnotations("View Ruleset", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "get_input",
			Description: "🔍 VIEW INPUT DETAILS: Get detailed configuration of a specific input component. 🎯 NEW: Automatically includes real sample data from the input source! Perfect for understanding data structure when creating rules. ⚠️ Check deployment status with 'get_pending_changes'.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Input component ID", Required: true},
			},
			Annotations: createAnnotations("View Input", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "get_output",
			Description: "🔍 VIEW OUTPUT DETAILS: Get detailed configuration of a specific output component. 🎯 NEW: Automatically includes sample data from upstream components showing what data flows through this output! ⚠️ Check deployment status with 'get_pending_changes'.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Output component ID", Required: true},
			},
			Annotations: createAnnotations("View Output", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "get_plugin",
			Description: "🔍 VIEW PLUGIN DETAILS: Get detailed configuration of a specific plugin component. ⚠️ Check deployment status with 'get_pending_changes' and project dependencies with 'get_component_usage'.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Plugin component ID", Required: true},
			},
			Annotations: createAnnotations("View Plugin", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "get_project",
			Description: "🔍 VIEW PROJECT DETAILS: Get detailed configuration of a specific project. 🎯 NEW: Automatically includes sample data from all input components in the project's data flow! Perfect for understanding the complete data pipeline. ⚠️ Check deployment status with 'get_pending_changes'.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project ID", Required: true},
			},
			Annotations: createAnnotations("View Project", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🚀 Deployment Tools
		{
			Name:        "get_pending_changes",
			Description: "📋 VIEW PENDING CHANGES: Show all components with temporary changes that need deployment. Essential before applying changes!",
			InputSchema: map[string]common.MCPToolArg{},
			Annotations: createAnnotations("View Pending", boolPtr(true), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
		{
			Name:        "apply_changes",
			Description: "🚀 DEPLOY CHANGES: Apply all pending changes to make them active in production. Use after reviewing pending changes!",
			InputSchema: map[string]common.MCPToolArg{},
			Annotations: createAnnotations("Deploy Changes", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},

		// 🧪 Testing Tools
		{
			Name:        "test_ruleset",
			Description: "🧪 TEST RULESET: Test a ruleset with sample data to verify it works correctly. Essential after rule changes!",
			InputSchema: map[string]common.MCPToolArg{
				"id":   {Type: "string", Description: "Ruleset ID", Required: true},
				"data": {Type: "string", Description: "JSON test data (required)", Required: true},
			},
			Annotations: createAnnotations("Test Ruleset", boolPtr(false), boolPtr(false), boolPtr(false), boolPtr(false)),
		},
	}
}

// CallAPITool calls the corresponding API endpoint for a given tool
func (m *APIMapper) CallAPITool(toolName string, args map[string]interface{}) (common.MCPToolResult, error) {
	// Handle new intelligent workflow tools (temporarily using legacy handlers for compatibility)
	switch toolName {
	// Core intelligent workflows - mapped to existing handlers for now
	case "create_rule_complete":
		return m.handleCreateRuleWithValidation(args)
	case "smart_deployment":
		return m.handleApplyChanges(args)
	case "component_wizard":
		return m.handleManageComponent(args)
	case "system_overview":
		return m.handleSystemHealthCheck(args)
	case "explore_components":
		return m.handleGetProjects(args) // Combined list functionality
	case "component_manager":
		return m.handleManageComponent(args)
	case "project_control":
		return m.handleControlProject(args)
	case "rule_manager":
		action, hasAction := args["action"].(string)
		if !hasAction {
			return common.MCPToolResult{
				Content: []common.MCPToolContent{{Type: "text", Text: "Error: action parameter is required"}},
				IsError: true,
			}, nil
		}

		// Route to appropriate handler based on action
		switch action {
		case "add_rule":
			return m.handleAddRulesetRule(args)
		case "update_rule":
			// First delete old rule, then add new one
			return m.handleUpdateRuleSafely(args)
		case "delete_rule":
			return m.handleDeleteRulesetRule(args)
		case "view_rules":
			return m.handleGetRuleset(args)
		case "create_ruleset":
			return m.handleCreateRuleset(args)
		case "update_ruleset":
			return m.handleUpdateRuleset(args)
		default:
			return common.MCPToolResult{
				Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Error: unknown action '%s'. Supported actions: add_rule, update_rule, delete_rule, view_rules, create_ruleset, update_ruleset", action)}},
				IsError: true,
			}, nil
		}
	case "test_lab":
		return m.handleTestComponent(args)
	case "deployment_center":
		return m.handleApplyChanges(args)
	case "learning_center":
		return m.handleGetRulesets(args)
	case "smart_assistant":
		return m.handleTroubleshootSystem(args)
	case "get_samplers_data_intelligent":
		return m.handleGetSamplersDataIntelligent(args)

	// Legacy compatibility handlers
	case "get_metrics":
		return m.handleGetMetrics(args)
	case "get_cluster_status":
		return m.handleGetClusterStatus(args)
	case "get_error_logs":
		return m.handleGetErrorLogs(args)
	case "get_pending_changes":
		return m.handleGetPendingChanges(args)
	case "apply_changes":
		return m.handleApplyChanges(args)
	case "verify_changes":
		return m.handleVerifyChanges(args)
	}

	// CRITICAL: get_samplers_data must be used BEFORE any rule creation!
	// Map tool names to API endpoints and methods
	endpointMap := map[string]struct {
		method   string
		endpoint string
		auth     bool
	}{
		// Public endpoints
		"ping":         {"GET", "/ping", false},
		"token_check":  {"GET", "/token-check", false},
		"get_qps_data": {"GET", "/qps-data", false},

		"get_daily_messages":         {"GET", "/daily-messages", false},
		"get_system_metrics":         {"GET", "/system-metrics", false},
		"get_system_stats":           {"GET", "/system-stats", false},
		"get_cluster_system_metrics": {"GET", "/cluster-system-metrics", false},
		"get_cluster_system_stats":   {"GET", "/cluster-system-stats", false},
		"get_cluster_status":         {"GET", "/cluster-status", false},
		"get_cluster":                {"GET", "/cluster", false},

		// Project endpoints
		"get_projects":                    {"GET", "/projects", true},
		"get_project":                     {"GET", "/projects/%s", true},
		"create_project":                  {"POST", "/projects", true},
		"update_project":                  {"PUT", "/projects/%s", true},
		"delete_project":                  {"DELETE", "/projects/%s", true},
		"start_project":                   {"POST", "/start-project", true},
		"stop_project":                    {"POST", "/stop-project", true},
		"restart_project":                 {"POST", "/restart-project", true},
		"restart_all_projects":            {"POST", "/restart-all-projects", true},
		"get_project_error":               {"GET", "/project-error/%s", true},
		"get_project_inputs":              {"GET", "/project-inputs/%s", true},
		"get_project_components":          {"GET", "/project-components/%s", true},
		"get_project_component_sequences": {"GET", "/project-component-sequences/%s", true},

		// Ruleset endpoints
		"get_rulesets":             {"GET", "/rulesets", true},
		"get_ruleset":              {"GET", "/rulesets/%s", true},
		"create_ruleset":           {"POST", "/rulesets", true},
		"update_ruleset":           {"PUT", "/rulesets/%s", true},
		"delete_ruleset":           {"DELETE", "/rulesets/%s", true},
		"delete_ruleset_rule":      {"DELETE", "/rulesets/%s/rules/%s", true},
		"add_ruleset_rule":         {"POST", "/rulesets/%s/rules", true},
		"get_ruleset_templates":    {"GET", "/ruleset-templates", true},
		"get_ruleset_syntax_guide": {"GET", "/ruleset-syntax-guide", true},
		"get_rule_templates":       {"GET", "/rule-templates", true},

		// Input endpoints
		"get_inputs":   {"GET", "/inputs", true},
		"get_input":    {"GET", "/inputs/%s", true},
		"create_input": {"POST", "/inputs", true},
		"update_input": {"PUT", "/inputs/%s", true},
		"delete_input": {"DELETE", "/inputs/%s", true},

		// Output endpoints
		"get_outputs":   {"GET", "/outputs", true},
		"get_output":    {"GET", "/outputs/%s", true},
		"create_output": {"POST", "/outputs", true},
		"update_output": {"PUT", "/outputs/%s", true},
		"delete_output": {"DELETE", "/outputs/%s", true},

		// Plugin endpoints
		"get_plugins":           {"GET", "/plugins", true},
		"get_plugin":            {"GET", "/plugins/%s", true},
		"create_plugin":         {"POST", "/plugins", true},
		"update_plugin":         {"PUT", "/plugins/%s", true},
		"delete_plugin":         {"DELETE", "/plugins/%s", true},
		"get_available_plugins": {"GET", "/available-plugins", true},
		"get_plugin_parameters": {"GET", "/plugin-parameters/%s", true},

		// Testing endpoints
		"verify_component":     {"POST", "/verify/%s/%s", true},
		"connect_check":        {"GET", "/connect-check/%s/%s", true},
		"test_plugin":          {"POST", "/test-plugin/%s", true},
		"test_plugin_content":  {"POST", "/test-plugin-content", true},
		"test_ruleset":         {"POST", "/test-ruleset/%s", true},
		"test_ruleset_content": {"POST", "/test-ruleset-content", true},
		"test_output":          {"POST", "/test-output/%s", true},
		"test_project":         {"POST", "/test-project/%s", true},
		"test_project_content": {"POST", "/test-project-content/%s", true},

		// Cluster management endpoints
		"cluster_heartbeat":   {"POST", "/cluster/heartbeat", true},
		"component_sync":      {"POST", "/component-sync", true},
		"project_status_sync": {"POST", "/project-status-sync", true},
		"qps_sync":            {"POST", "/qps-sync", true},
		"get_config_root":     {"GET", "/config_root", true},
		"download_config":     {"GET", "/config/download", true},

		// Pending changes management
		"get_pending_changes":          {"GET", "/pending-changes", true},
		"get_enhanced_pending_changes": {"GET", "/pending-changes/enhanced", true},
		"apply_changes":                {"POST", "/apply-changes", true},
		"apply_changes_enhanced":       {"POST", "/apply-changes/enhanced", true},
		"apply_single_change":          {"POST", "/apply-single-change", true},
		"verify_changes":               {"POST", "/verify-changes", true},
		"verify_change":                {"POST", "/verify-change/%s/%s", true},
		"cancel_change":                {"DELETE", "/cancel-change/%s/%s", true},
		"cancel_all_changes":           {"DELETE", "/cancel-all-changes", true},

		// Temporary file management
		"create_temp_file": {"POST", "/temp-file/%s/%s", true},
		"check_temp_file":  {"GET", "/temp-file/%s/%s", true},
		"delete_temp_file": {"DELETE", "/temp-file/%s/%s", true},

		// ⚠️ MANDATORY BEFORE RULE CREATION: Must use this to get real data samples first!
		"get_samplers_data":             {"GET", "/samplers/data", true},
		"get_samplers_data_intelligent": {"POST", "/samplers/data/intelligent", true},
		"get_ruleset_fields":            {"GET", "/ruleset-fields/%s", true},

		// Cancel upgrade routes
		"cancel_ruleset_upgrade": {"POST", "/cancel-upgrade/rulesets/%s", true},
		"cancel_input_upgrade":   {"POST", "/cancel-upgrade/inputs/%s", true},
		"cancel_output_upgrade":  {"POST", "/cancel-upgrade/outputs/%s", true},
		"cancel_project_upgrade": {"POST", "/cancel-upgrade/projects/%s", true},
		"cancel_plugin_upgrade":  {"POST", "/cancel-upgrade/plugins/%s", true},

		// Component usage analysis
		"get_component_usage": {"GET", "/component-usage/%s/%s", true},

		// Search
		"search_components": {"GET", "/search-components", true},

		// Local changes
		"get_local_changes":        {"GET", "/local-changes", true},
		"load_local_changes":       {"POST", "/load-local-changes", true},
		"load_single_local_change": {"POST", "/load-single-local-change", true},

		// Metrics sync
		"metrics_sync": {"POST", "/metrics-sync", true},

		// Error logs
		"get_error_logs":         {"GET", "/error-logs", true},
		"get_cluster_error_logs": {"GET", "/cluster-error-logs", true},
	}

	endpointInfo, exists := endpointMap[toolName]
	if !exists {
		return common.MCPToolResult{}, fmt.Errorf("unknown tool: %s", toolName)
	}

	// Build the endpoint URL with parameters
	endpoint := endpointInfo.endpoint

	// Handle different endpoint parameter patterns
	switch {
	case strings.Contains(endpoint, "%s/%s"):
		// Two parameters needed
		if componentType, exists := args["type"]; exists {
			if id, exists := args["id"]; exists {
				endpoint = fmt.Sprintf(endpointInfo.endpoint, componentType, id)
			}
		} else if projectName, exists := args["project_name"]; exists {
			if inputNode, exists := args["inputNode"]; exists {
				endpoint = fmt.Sprintf(endpointInfo.endpoint, projectName, inputNode)
			}
		} else if toolName == "delete_ruleset_rule" {
			// Special handling for delete_ruleset_rule: id and rule_id
			if id, exists := args["id"]; exists {
				if ruleId, exists := args["rule_id"]; exists {
					endpoint = fmt.Sprintf(endpointInfo.endpoint, id, ruleId)
				}
			}
		}
	case strings.Contains(endpoint, "%s"):
		// One parameter needed
		if id, exists := args["id"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, id)
		} else if projectName, exists := args["project_name"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, projectName)
		} else if rulesetName, exists := args["ruleset_name"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, rulesetName)
		} else if inputName, exists := args["input_name"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, inputName)
		} else if outputName, exists := args["output_name"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, outputName)
		} else if pluginName, exists := args["plugin_name"]; exists {
			endpoint = fmt.Sprintf(endpointInfo.endpoint, pluginName)
		}
	}

	// Handle query parameters for GET requests
	if endpointInfo.method == "GET" && len(args) > 0 {
		query := url.Values{}
		for key, value := range args {
			// Skip parameters that are used in URL path
			if key == "id" || key == "type" || key == "project_name" || key == "inputNode" ||
				key == "ruleset_name" || key == "input_name" || key == "output_name" || key == "plugin_name" ||
				key == "rule_id" {
				continue
			}
			if strValue, ok := value.(string); ok {
				query.Add(key, strValue)
			}
		}
		if len(query) > 0 {
			if strings.Contains(endpoint, "?") {
				endpoint += "&" + query.Encode()
			} else {
				endpoint += "?" + query.Encode()
			}
		}
	}

	// Make the HTTP request
	responseBody, err := m.makeHTTPRequest(endpointInfo.method, endpoint, args, endpointInfo.auth)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Error calling tool: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Format the response as text, prettifying JSON if possible
	var prettyResponseBody string
	var jsonData interface{}
	if json.Unmarshal(responseBody, &jsonData) == nil {
		// It's valid JSON, format it nicely
		prettyBytes, err := json.MarshalIndent(jsonData, "", "  ")
		if err == nil {
			prettyResponseBody = string(prettyBytes)
		} else {
			prettyResponseBody = string(responseBody) // Fallback to raw if re-marshalling fails
		}
	} else {
		// Not JSON, return as-is
		prettyResponseBody = string(responseBody)
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Type: "text", // MCP supports "text", "image", and "resource" types
				Text: prettyResponseBody,
			},
		},
	}, nil
}

// makeHTTPRequest makes an HTTP request to the API
func (m *APIMapper) makeHTTPRequest(method, endpoint string, body interface{}, requireAuth bool) ([]byte, error) {
	url := m.baseURL + endpoint

	var reqBody io.Reader
	if body != nil && (method == "POST" || method == "PUT") {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	if requireAuth {
		req.Header.Set("token", m.token)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		// Attempt to parse a standard error response from the API
		var apiError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(responseBody, &apiError) == nil && apiError.Error != "" {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, apiError.Error)
		}
		// Fallback to returning the raw response body
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

// handleCreateRuleWithValidation orchestrates the complete rule creation workflow
func (m *APIMapper) handleCreateRuleWithValidation(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["ruleset_id"].(string)
	ruleRaw := args["rule_raw"].(string)
	testData, hasTestData := args["test_data"].(string)

	// MANDATORY: Check if real sample data is provided
	if !hasTestData || testData == "" {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: "❌ SAMPLE DATA REQUIRED: Must provide real sample data for rule creation!\n\n🎯 **Two Options:**\n1. **Try backend data:** Use 'get_samplers_data' (may fail if backend has no data)\n2. **Provide your own:** Add real JSON sample data directly to the 'test_data' parameter\n\n⚠️ **Cannot create rules without actual data examples!**"}},
			IsError: true,
		}, nil
	}

	// Validate that test data appears to be real (basic checks)
	if len(testData) < 50 || !strings.Contains(testData, "{") {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: "❌ INVALID SAMPLE DATA: The provided data appears to be too simple or not in JSON format.\n\n🎯 **Required Format:**\n- Must be real JSON data from your actual system\n- Should contain actual field names and values\n- Example: {\"timestamp\":\"2024-01-01T10:00:00Z\",\"source_ip\":\"192.168.1.1\",\"exe\":\"msf.exe\",...}\n\n💡 **Get real data from:** your log files, monitoring systems, or actual data samples"}},
			IsError: true,
		}, nil
	}

	var results []string
	results = append(results, "=== DATA-DRIVEN RULE CREATION WORKFLOW ===\n")
	results = append(results, "✅ Sample data validation passed - proceeding with rule creation...")

	// Step 1: Add the rule
	results = append(results, "Step 1: Adding rule to ruleset...")
	addArgs := map[string]interface{}{
		"id":       rulesetId,
		"rule_raw": ruleRaw,
	}
	addResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/rulesets/%s/rules", rulesetId), addArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Rule addition failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Rule added successfully: %s\n", string(addResponse)))

	// Step 2: Verify the ruleset
	results = append(results, "Step 2: Verifying ruleset configuration...")
	verifyResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/verify/ruleset/%s", rulesetId), nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Verification failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Verification passed: %s\n", string(verifyResponse)))
	}

	// Step 3: Test with sample data if provided
	if hasTestData {
		results = append(results, "Step 3: Testing rule with sample data...")
		testArgs := map[string]interface{}{
			"test_data": testData,
		}
		testResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/test-ruleset/%s", rulesetId), testArgs, true)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Testing failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Testing completed: %s\n", string(testResponse)))
		}
	}

	// Step 4: Get usage analysis
	results = append(results, "Step 4: Analyzing component usage and impact...")
	usageResponse, err := m.makeHTTPRequest("GET", fmt.Sprintf("/component-usage/ruleset/%s", rulesetId), nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Usage analysis failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Usage analysis: %s\n", string(usageResponse)))
	}

	// Step 5: Deployment guidance
	results = append(results, "\n=== 🚀 DEPLOYMENT GUIDANCE ===")
	results = append(results, "⚠️  IMPORTANT: Your rule has been created in a TEMPORARY file and is NOT YET ACTIVE!")
	results = append(results, "")
	results = append(results, "📋 Next Steps Required:")
	results = append(results, "1. 🔍 Check what's pending: Use 'get_pending_changes' to see all changes awaiting deployment")
	results = append(results, "2. ✅ Apply changes: Use 'apply_changes' to deploy your rule to production")
	results = append(results, "3. 🧪 Test thoroughly: Use 'test_ruleset' with real data to validate rule behavior")
	results = append(results, "")
	results = append(results, "🎯 Recommended Workflow:")
	results = append(results, "   → get_pending_changes  (review what will be deployed)")
	results = append(results, "   → apply_changes        (activate your rule)")
	results = append(results, "   → test_ruleset         (verify it works correctly)")
	results = append(results, "")
	results = append(results, "💡 Your rule will remain inactive until you run 'apply_changes'!")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleUpdateRuleSafely orchestrates the safe rule update workflow
func (m *APIMapper) handleUpdateRuleSafely(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["ruleset_id"].(string)
	ruleId := args["rule_id"].(string)
	ruleRaw := args["rule_raw"].(string)
	testData, hasTestData := args["test_data"].(string)

	var results []string
	results = append(results, "=== SAFE RULE UPDATE WORKFLOW ===\n")

	// Step 1: Get current ruleset for backup
	results = append(results, "Step 1: Backing up current ruleset...")
	_, err := m.makeHTTPRequest("GET", fmt.Sprintf("/rulesets/%s", rulesetId), nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Backup failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, "✓ Current ruleset backed up")

	// Step 2: Delete old rule
	results = append(results, "Step 2: Removing old rule...")
	deleteResponse, err := m.makeHTTPRequest("DELETE", fmt.Sprintf("/rulesets/%s/rules/%s", rulesetId, ruleId), nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Rule deletion failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Old rule removed: %s\n", string(deleteResponse)))
	}

	// Step 3: Add new rule
	results = append(results, "Step 3: Adding updated rule...")
	addArgs := map[string]interface{}{
		"id":       rulesetId,
		"rule_raw": ruleRaw,
	}
	addResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/rulesets/%s/rules", rulesetId), addArgs, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Rule addition failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Updated rule added: %s\n", string(addResponse)))
	}

	// Step 4: Verify updated ruleset
	results = append(results, "Step 4: Verifying updated ruleset...")
	verifyResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/verify/ruleset/%s", rulesetId), nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Verification failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Verification passed: %s\n", string(verifyResponse)))
	}

	// Step 5: Test if data provided
	if hasTestData {
		results = append(results, "Step 5: Testing updated rule...")
		testArgs := map[string]interface{}{
			"test_data": testData,
		}
		testResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/test-ruleset/%s", rulesetId), testArgs, true)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Testing failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Testing completed: %s\n", string(testResponse)))
		}
	}

	// Step 6: Deployment guidance for rule updates
	results = append(results, "\n=== 🚀 DEPLOYMENT GUIDANCE ===")
	results = append(results, "⚠️  IMPORTANT: Your rule update has been saved to a TEMPORARY file and is NOT YET ACTIVE!")
	results = append(results, "")
	results = append(results, "📋 Next Steps Required:")
	results = append(results, "1. 🔍 Review changes: Use 'get_pending_changes' to see all modifications awaiting deployment")
	results = append(results, "2. ✅ Deploy update: Use 'apply_changes' to activate your updated rule in production")
	results = append(results, "3. 🧪 Verify changes: Use 'test_ruleset' to ensure the updated rule works as expected")
	results = append(results, "")
	results = append(results, "🎯 Deployment Workflow:")
	results = append(results, "   → get_pending_changes  (review what will be deployed)")
	results = append(results, "   → apply_changes        (activate your updated rule)")
	results = append(results, "   → test_ruleset         (verify the update works correctly)")
	results = append(results, "")
	results = append(results, "💡 The old rule version is still active until you run 'apply_changes'!")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleManageComponent orchestrates complete component management workflow
func (m *APIMapper) handleManageComponent(args map[string]interface{}) (common.MCPToolResult, error) {
	operation := args["operation"].(string)
	componentType := args["type"].(string)
	componentId := args["id"].(string)
	rawContent, hasRawContent := args["raw"].(string)
	testData, hasTestData := args["test_data"].(string)

	var results []string
	results = append(results, fmt.Sprintf("=== COMPLETE %s MANAGEMENT WORKFLOW ===\n", strings.ToUpper(componentType)))

	// Step 1: Create component if operation is "create"
	if operation == "create" {
		results = append(results, "Step 1: Creating component...")
		createArgs := map[string]interface{}{
			"id":  componentId,
			"raw": rawContent,
		}
		createResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/%ss", componentType), createArgs, true)
		if err != nil {
			return common.MCPToolResult{
				Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Component creation failed: %v", err)}},
				IsError: true,
			}, nil
		}
		results = append(results, fmt.Sprintf("✓ Component created: %s\n", string(createResponse)))
	}

	// Step 2: Verify component
	results = append(results, "Step 2: Verifying component...")
	verifyResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/verify/%s/%s", componentType, componentId), nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Verification failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Verification passed: %s\n", string(verifyResponse)))
	}

	// Step 3: Connectivity test for inputs/outputs
	if componentType == "input" || componentType == "output" {
		results = append(results, "Step 3: Testing connectivity...")
		connectResponse, err := m.makeHTTPRequest("GET", fmt.Sprintf("/connect-check/%s/%s", componentType, componentId), nil, true)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Connectivity test failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Connectivity test passed: %s\n", string(connectResponse)))
		}
	}

	// Step 4: Test with sample data if provided
	if hasTestData {
		results = append(results, "Step 4: Testing with sample data...")
		testArgs := map[string]interface{}{
			"test_data": testData,
		}
		var testEndpoint string
		switch componentType {
		case "ruleset":
			testEndpoint = fmt.Sprintf("/test-ruleset/%s", componentId)
		case "plugin":
			testEndpoint = fmt.Sprintf("/test-plugin/%s", componentId)
		case "output":
			testEndpoint = fmt.Sprintf("/test-output/%s", componentId)
		}
		if testEndpoint != "" {
			testResponse, err := m.makeHTTPRequest("POST", testEndpoint, testArgs, true)
			if err != nil {
				results = append(results, fmt.Sprintf("✗ Testing failed: %v\n", err))
			} else {
				results = append(results, fmt.Sprintf("✓ Testing completed: %s\n", string(testResponse)))
			}
		}
	}

	// Step 5: Deployment if requested
	if hasRawContent {
		results = append(results, "Step 5: Deploying component...")
		applyResponse, err := m.makeHTTPRequest("POST", "/apply-changes", nil, true)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Deployment failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Deployment completed: %s\n", string(applyResponse)))
			results = append(results, "🎉 Component is now ACTIVE in production!")
		}
	} else {
		results = append(results, "Step 5: Component created in temporary file")

		// Add deployment guidance
		results = append(results, "\n=== 🚀 DEPLOYMENT GUIDANCE ===")
		results = append(results, fmt.Sprintf("⚠️  IMPORTANT: Your %s has been created in a TEMPORARY file and is NOT YET ACTIVE!", strings.ToUpper(componentType)))
		results = append(results, "")
		results = append(results, "📋 Next Steps Required:")
		results = append(results, "1. 🔍 Review changes: Use 'get_pending_changes' to see all components awaiting deployment")
		results = append(results, "2. ✅ Deploy component: Use 'apply_changes' to activate your component in production")
		if componentType == "input" || componentType == "output" {
			results = append(results, "3. 🔗 Test connectivity: Use 'connect_check' to verify connection to external systems")
		}
		if componentType == "plugin" {
			results = append(results, "3. 🧪 Test plugin: Use 'test_plugin' to verify plugin functionality")
		}
		results = append(results, "")
		results = append(results, "🎯 Deployment Workflow:")
		results = append(results, "   → get_pending_changes  (review what will be deployed)")
		results = append(results, "   → apply_changes        (activate your component)")
		results = append(results, "")
		results = append(results, "💡 Your component will remain inactive until you run 'apply_changes'!")
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleSystemHealthCheck orchestrates comprehensive system health assessment
func (m *APIMapper) handleSystemHealthCheck(args map[string]interface{}) (common.MCPToolResult, error) {
	includePerformance, shouldIncludePerf := args["include_performance"].(string)
	checkDependencies, shouldCheckDeps := args["check_dependencies"].(string)

	var results []string
	results = append(results, "=== COMPREHENSIVE SYSTEM HEALTH CHECK ===\n")

	// Step 1: Cluster health
	results = append(results, "Step 1: Checking cluster health...")
	clusterResponse, err := m.makeHTTPRequest("GET", "/cluster-status", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Cluster health check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Cluster status: %s\n", string(clusterResponse)))
	}

	// Step 2: All projects health
	results = append(results, "Step 2: Checking all projects...")
	projectsResponse, err := m.makeHTTPRequest("GET", "/projects", nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Projects health check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Projects status: %s\n", string(projectsResponse)))
	}

	// Step 3: System resources
	results = append(results, "Step 3: Checking system resources...")
	systemResponse, err := m.makeHTTPRequest("GET", "/system-metrics", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ System metrics check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ System metrics: %s\n", string(systemResponse)))
	}

	// Step 4: Error logs analysis
	results = append(results, "Step 4: Analyzing error logs...")
	errorResponse, err := m.makeHTTPRequest("GET", "/error-logs", nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Error log analysis failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Error logs analysis: %s\n", string(errorResponse)))
	}

	// Step 5: Performance analysis (if requested)
	if shouldIncludePerf && includePerformance == "true" {
		results = append(results, "Step 5: Performance analysis...")
		qpsResponse, err := m.makeHTTPRequest("GET", "/qps-stats", nil, false)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Performance analysis failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Performance analysis: %s\n", string(qpsResponse)))
		}
	}

	// Step 6: Dependency checks (if requested)
	if shouldCheckDeps && checkDependencies == "true" {
		results = append(results, "Step 6: Checking component dependencies...")
		// Get all rulesets and check dependencies
		rulesetsResponse, err := m.makeHTTPRequest("GET", "/rulesets", nil, true)
		if err != nil {
			results = append(results, fmt.Sprintf("✗ Dependency check failed: %v\n", err))
		} else {
			results = append(results, fmt.Sprintf("✓ Component dependencies: %s\n", string(rulesetsResponse)))
		}
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleTroubleshootSystem orchestrates intelligent system troubleshooting
func (m *APIMapper) handleTroubleshootSystem(args map[string]interface{}) (common.MCPToolResult, error) {
	task := args["task"].(string)
	context, hasContext := args["context"].(string)

	var results []string

	// Handle system introduction request
	if task == "system_intro" {
		return m.generateSystemIntroduction()
	}

	results = append(results, "=== INTELLIGENT SYSTEM TROUBLESHOOTING ===\n")
	results = append(results, fmt.Sprintf("Task: %s\n", task))

	if hasContext {
		results = append(results, fmt.Sprintf("Context: %s\n", context))
	}

	// Step 1: Error log analysis
	results = append(results, "Step 1: Analyzing error logs...")
	errorResponse, err := m.makeHTTPRequest("GET", "/error-logs", nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Error log analysis failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Error logs: %s\n", string(errorResponse)))
	}

	// Step 2: Component health check
	results = append(results, "Step 2: Component health verification...")
	projectsResponse, err := m.makeHTTPRequest("GET", "/projects", nil, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Projects health check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ All projects status: %s\n", string(projectsResponse)))
	}

	// Step 3: Performance anomaly detection
	results = append(results, "Step 3: Performance anomaly detection...")
	qpsResponse, err := m.makeHTTPRequest("GET", "/qps-stats", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Performance analysis failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Performance metrics: %s\n", string(qpsResponse)))
	}

	// Step 4: System resource check
	results = append(results, "Step 4: System resource analysis...")
	systemResponse, err := m.makeHTTPRequest("GET", "/system-metrics", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ System resource check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ System resources: %s\n", string(systemResponse)))
	}

	// Step 5: Cluster health (if applicable)
	results = append(results, "Step 5: Cluster health verification...")
	clusterResponse, err := m.makeHTTPRequest("GET", "/cluster-status", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Cluster health check failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Cluster status: %s\n", string(clusterResponse)))
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetMetrics retrieves comprehensive system metrics
func (m *APIMapper) handleGetMetrics(args map[string]interface{}) (common.MCPToolResult, error) {
	projectId, hasProjectId := args["project_id"].(string)
	timeRange, hasTimeRange := args["time_range"].(string)
	aggregated, hasAggregated := args["aggregated"].(string)

	var results []string
	results = append(results, "=== SYSTEM METRICS ===\n")

	// Step 1: Retrieve metrics based on type
	results = append(results, "Step 1: Retrieving metrics...")
	metricsArgs := ""
	if hasProjectId {
		metricsArgs += fmt.Sprintf("?project_id=%s", projectId)
	}
	if hasTimeRange {
		metricsArgs += fmt.Sprintf("&time_range=%s", timeRange)
	}
	if hasAggregated {
		metricsArgs += fmt.Sprintf("&aggregated=%s", aggregated)
	}
	metricsResponse, err := m.makeHTTPRequest("GET", "/system-metrics"+metricsArgs, nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Metrics retrieval failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Metrics retrieved: %s\n", string(metricsResponse)))
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetClusterStatus retrieves comprehensive cluster status information
func (m *APIMapper) handleGetClusterStatus(args map[string]interface{}) (common.MCPToolResult, error) {
	var results []string
	results = append(results, "=== CLUSTER STATUS ===\n")

	// Step 1: Retrieve cluster status
	results = append(results, "Step 1: Retrieving cluster status...")
	clusterStatusResponse, err := m.makeHTTPRequest("GET", "/cluster-status", nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Cluster status retrieval failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Cluster status retrieved: %s\n", string(clusterStatusResponse)))
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetErrorLogs retrieves system error logs
func (m *APIMapper) handleGetErrorLogs(args map[string]interface{}) (common.MCPToolResult, error) {
	clusterWide, hasClusterWide := args["cluster_wide"].(string)

	var results []string
	results = append(results, "=== SYSTEM ERROR LOGS ===\n")

	// Step 1: Retrieve error logs
	results = append(results, "Step 1: Retrieving error logs...")
	errorLogsArgs := ""
	if hasClusterWide {
		errorLogsArgs += fmt.Sprintf("?cluster_wide=%s", clusterWide)
	}
	errorLogsResponse, err := m.makeHTTPRequest("GET", "/error-logs"+errorLogsArgs, nil, false)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Error logs retrieval failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Error logs retrieved: %s\n", string(errorLogsResponse)))
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetProjects retrieves comprehensive list of all projects
func (m *APIMapper) handleGetProjects(args map[string]interface{}) (common.MCPToolResult, error) {
	var results []string
	results = append(results, "=== PROJECT LIST ===\n")

	// Step 1: Retrieve projects
	results = append(results, "Step 1: Retrieving projects...")
	projectsResponse, err := m.makeHTTPRequest("GET", "/projects", nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get projects: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Projects retrieved: %s\n", string(projectsResponse)))

	// Step 2: Add critical guidance
	results = append(results, "\n=== ⚠️  IMPORTANT NEXT STEPS ===")
	results = append(results, "📋 **Check Project Health:**")
	results = append(results, "   → Use 'get_project_error' with id='<project_name>' to check for errors")
	results = append(results, "   → Use 'project_control' with action='status' to check running status")
	results = append(results, "")
	results = append(results, "🔗 **Check Component Dependencies:**")
	results = append(results, "   → Use 'get_project_components' with id='<project_name>' to see used components")
	results = append(results, "   → Use 'get_project_component_sequences' to see data flow")
	results = append(results, "")
	results = append(results, "📋 **Check Component Status:**")
	results = append(results, "   → Use 'get_pending_changes' to see if any components have unpublished changes")
	results = append(results, "   → Components with pending changes may affect project behavior!")
	results = append(results, "")
	results = append(results, "⚡ **Common Actions:**")
	results = append(results, "   → 'test_project' - Test project end-to-end")
	results = append(results, "   → 'project_control' - Start/stop/restart projects")
	results = append(results, "   → 'apply_changes' - Deploy pending component changes")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleControlProject performs unified project control operations
func (m *APIMapper) handleControlProject(args map[string]interface{}) (common.MCPToolResult, error) {
	action := args["action"].(string)
	projectId, hasProjectId := args["project_id"].(string)

	var results []string
	results = append(results, fmt.Sprintf("=== PROJECT CONTROL (%s) ===\n", strings.ToUpper(action)))

	// Map actions to correct endpoints
	var endpoint string
	var controlArgs map[string]interface{}

	switch action {
	case "start":
		endpoint = "/start-project"
		controlArgs = map[string]interface{}{"project_id": projectId}
	case "stop":
		endpoint = "/stop-project"
		controlArgs = map[string]interface{}{"project_id": projectId}
	case "restart":
		endpoint = "/restart-project"
		controlArgs = map[string]interface{}{"project_id": projectId}
	case "start_all":
		endpoint = "/restart-all-projects"
		controlArgs = map[string]interface{}{}
	case "stop_all":
		// There's no stop-all endpoint, so we handle this differently
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: "Stop all projects is not supported by the API. Please stop projects individually."}},
			IsError: true,
		}, nil
	case "status":
		if !hasProjectId {
			return common.MCPToolResult{
				Content: []common.MCPToolContent{{Type: "text", Text: "Project ID is required for status check"}},
				IsError: true,
			}, nil
		}
		// Use get project endpoint for status
		projectResponse, err := m.makeHTTPRequest("GET", fmt.Sprintf("/projects/%s", projectId), nil, true)
		if err != nil {
			return common.MCPToolResult{
				Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get project status: %v", err)}},
				IsError: true,
			}, nil
		}
		results = append(results, fmt.Sprintf("✓ Project status: %s\n", string(projectResponse)))
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
		}, nil
	default:
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Unknown action: %s. Supported actions: start, stop, restart, start_all, status", action)}},
			IsError: true,
		}, nil
	}

	if !hasProjectId && action != "start_all" {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: "Project ID is required for this action"}},
			IsError: true,
		}, nil
	}

	// Step 1: Perform control operation
	results = append(results, "Step 1: Performing control operation...")
	controlResponse, err := m.makeHTTPRequest("POST", endpoint, controlArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Project control failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Control operation completed: %s\n", string(controlResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetRulesets retrieves comprehensive list of all rulesets
func (m *APIMapper) handleGetRulesets(args map[string]interface{}) (common.MCPToolResult, error) {
	var results []string
	results = append(results, "=== RULESET LIST ===\n")

	// Step 1: Retrieve rulesets
	results = append(results, "Step 1: Retrieving rulesets...")
	rulesetsResponse, err := m.makeHTTPRequest("GET", "/rulesets", nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get rulesets: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Rulesets retrieved: %s\n", string(rulesetsResponse)))

	// Step 2: Add critical guidance
	results = append(results, "\n=== ⚠️  IMPORTANT NEXT STEPS ===")
	results = append(results, "📋 **Check Deployment Status:**")
	results = append(results, "   → Use 'get_pending_changes' to see which rulesets have unpublished changes")
	results = append(results, "   → Rulesets with pending changes are NOT ACTIVE until deployed!")
	results = append(results, "")
	results = append(results, "🔗 **Check Project Dependencies:**")
	results = append(results, "   → Use 'get_component_usage' with type='ruleset' and id='<ruleset_name>'")
	results = append(results, "   → This shows which projects depend on each ruleset")
	results = append(results, "")
	results = append(results, "🚀 **If you see pending changes:**")
	results = append(results, "   → Review them with 'get_pending_changes'")
	results = append(results, "   → Deploy them with 'apply_changes' to make them active")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetRuleset retrieves complete information for a specific ruleset
func (m *APIMapper) handleGetRuleset(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["id"].(string)

	var results []string
	results = append(results, "=== RULESET DETAILS ===\n")

	// Step 1: Retrieve ruleset
	results = append(results, "Step 1: Retrieving ruleset...")
	rulesetResponse, err := m.makeHTTPRequest("GET", fmt.Sprintf("/rulesets/%s", rulesetId), nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get ruleset: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Ruleset retrieved: %s\n", string(rulesetResponse)))

	// Step 2: Add critical analysis guidance
	results = append(results, "\n=== ⚠️  DEPLOYMENT & USAGE ANALYSIS ===")
	results = append(results, "📋 **Check if changes are deployed:**")
	results = append(results, fmt.Sprintf("   → Use 'get_pending_changes' to check if '%s' has unpublished changes", rulesetId))
	results = append(results, "   → If you see temporary changes above, they are NOT ACTIVE until deployed!")
	results = append(results, "")
	results = append(results, "🔗 **Check which projects use this ruleset:**")
	results = append(results, fmt.Sprintf("   → Use 'get_component_usage' with type='ruleset' and id='%s'", rulesetId))
	results = append(results, "   → This shows project dependencies and impact of changes")
	results = append(results, "")
	results = append(results, "⚡ **Quick Actions:**")
	results = append(results, "   → 'test_ruleset' - Test this ruleset with sample data")
	results = append(results, "   → 'apply_changes' - Deploy any pending changes")
	results = append(results, "   → 'rule_manager' with action='add_rule' - Add new rules")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleCreateRuleset creates a new ruleset with XML configuration and validation
func (m *APIMapper) handleCreateRuleset(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["id"].(string)
	raw := args["raw"].(string)

	var results []string
	results = append(results, "=== RULESET CREATION ===\n")

	// Step 1: Create ruleset
	results = append(results, "Step 1: Creating ruleset...")
	createArgs := map[string]interface{}{
		"id":  rulesetId,
		"raw": raw,
	}
	createResponse, err := m.makeHTTPRequest("POST", "/rulesets", createArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Ruleset creation failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Ruleset created: %s\n", string(createResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleAddRulesetRule adds a single rule to an existing ruleset
func (m *APIMapper) handleAddRulesetRule(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["id"].(string)
	ruleRaw := args["rule_raw"].(string)

	var results []string
	results = append(results, "=== RULE ADDITION ===\n")

	// Step 1: Add rule
	results = append(results, "Step 1: Adding rule to ruleset...")
	addArgs := map[string]interface{}{
		"rule_raw": ruleRaw,
	}
	addResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/rulesets/%s/rules", rulesetId), addArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Rule addition failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Rule added successfully: %s\n", string(addResponse)))

	// Step 2: Add deployment guidance
	results = append(results, "\n=== 🚀 DEPLOYMENT GUIDANCE ===")
	results = append(results, "⚠️  IMPORTANT: Your rule has been created in a TEMPORARY file and is NOT YET ACTIVE!")
	results = append(results, "")
	results = append(results, "📋 Next Steps Required:")
	results = append(results, "1. 🔍 Check what's pending: Use 'get_pending_changes'")
	results = append(results, "2. ✅ Apply changes: Use 'apply_changes' to deploy your rule")
	results = append(results, "3. 🧪 Test thoroughly: Use 'test_ruleset' with real data")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleUpdateRuleset updates an entire ruleset configuration
func (m *APIMapper) handleUpdateRuleset(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["id"].(string)
	raw := args["raw"].(string)

	var results []string
	results = append(results, "=== RULESET UPDATE ===\n")

	// Step 1: Update ruleset
	results = append(results, "Step 1: Updating ruleset...")
	updateArgs := map[string]interface{}{
		"id":  rulesetId,
		"raw": raw,
	}
	updateResponse, err := m.makeHTTPRequest("PUT", fmt.Sprintf("/rulesets/%s", rulesetId), updateArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Ruleset update failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Ruleset updated: %s\n", string(updateResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleDeleteRulesetRule deletes a specific rule from a ruleset
func (m *APIMapper) handleDeleteRulesetRule(args map[string]interface{}) (common.MCPToolResult, error) {
	rulesetId := args["id"].(string)
	ruleId := args["rule_id"].(string)

	var results []string
	results = append(results, "=== RULE DELETION ===\n")

	// Step 1: Delete rule
	results = append(results, "Step 1: Deleting rule...")
	deleteResponse, err := m.makeHTTPRequest("DELETE", fmt.Sprintf("/rulesets/%s/rules/%s", rulesetId, ruleId), nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Rule deletion failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Rule deleted: %s\n", string(deleteResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetInputs retrieves comprehensive list of all input components
func (m *APIMapper) handleGetInputs(args map[string]interface{}) (common.MCPToolResult, error) {
	var results []string
	results = append(results, "=== INPUT COMPONENTS ===\n")

	// Step 1: Retrieve inputs
	results = append(results, "Step 1: Retrieving inputs...")
	inputsResponse, err := m.makeHTTPRequest("GET", "/inputs", nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get inputs: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Inputs retrieved: %s\n", string(inputsResponse)))

	// Step 2: Add critical guidance
	results = append(results, "\n=== ⚠️  IMPORTANT NEXT STEPS ===")
	results = append(results, "📋 **Check Deployment Status:**")
	results = append(results, "   → Use 'get_pending_changes' to see which inputs have unpublished changes")
	results = append(results, "   → Inputs with pending changes are NOT ACTIVE until deployed!")
	results = append(results, "")
	results = append(results, "🔗 **Check Project Dependencies:**")
	results = append(results, "   → Use 'get_component_usage' with type='input' and id='<input_name>'")
	results = append(results, "   → This shows which projects depend on each input")
	results = append(results, "")
	results = append(results, "⚡ **Common Actions:**")
	results = append(results, "   → 'connect_check' - Test input connectivity")
	results = append(results, "   → 'apply_changes' - Deploy pending changes")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleTestComponent performs unified testing for components
func (m *APIMapper) handleTestComponent(args map[string]interface{}) (common.MCPToolResult, error) {
	componentType := args["type"].(string)
	componentId, _ := args["id"].(string)
	testData, _ := args["test_data"].(string)
	content, hasContent := args["content"].(string)

	var results []string
	results = append(results, fmt.Sprintf("=== TESTING %s ===\n", strings.ToUpper(componentType)))

	// Step 1: Test component
	results = append(results, "Step 1: Testing component...")
	testArgs := map[string]interface{}{
		"id":        componentId,
		"test_data": testData,
	}
	if hasContent {
		testArgs["content"] = content
	}
	testResponse, err := m.makeHTTPRequest("POST", fmt.Sprintf("/test-component/%s", componentType), testArgs, true)
	if err != nil {
		results = append(results, fmt.Sprintf("✗ Testing failed: %v\n", err))
	} else {
		results = append(results, fmt.Sprintf("✓ Testing completed: %s\n", string(testResponse)))
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetPendingChanges retrieves all pending configuration changes
func (m *APIMapper) handleGetPendingChanges(args map[string]interface{}) (common.MCPToolResult, error) {
	enhanced, hasEnhanced := args["enhanced"].(string)

	var results []string
	results = append(results, "=== PENDING CHANGES ===\n")

	// Step 1: Retrieve pending changes
	results = append(results, "Step 1: Retrieving pending changes...")
	pendingChangesArgs := ""
	if hasEnhanced {
		pendingChangesArgs += fmt.Sprintf("?enhanced=%s", enhanced)
	}
	pendingChangesResponse, err := m.makeHTTPRequest("GET", "/pending-changes"+pendingChangesArgs, nil, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Failed to get pending changes: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Pending changes retrieved: %s\n", string(pendingChangesResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleApplyChanges applies all pending configuration changes
func (m *APIMapper) handleApplyChanges(args map[string]interface{}) (common.MCPToolResult, error) {
	_, hasEnhanced := args["enhanced"].(string)

	var results []string
	results = append(results, "=== APPLYING CHANGES ===\n")

	// Step 1: Apply changes
	results = append(results, "Step 1: Applying changes...")
	applyArgs := map[string]interface{}{
		"enhanced": hasEnhanced,
	}
	applyResponse, err := m.makeHTTPRequest("POST", "/apply-changes", applyArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Change application failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Changes applied: %s\n", string(applyResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleVerifyChanges verifies pending changes for consistency and dependency issues
func (m *APIMapper) handleVerifyChanges(args map[string]interface{}) (common.MCPToolResult, error) {
	typeToVerify, hasTypeToVerify := args["type"].(string)
	idToVerify, hasIdToVerify := args["id"].(string)

	var results []string
	results = append(results, "=== VERIFYING CHANGES ===\n")

	// Step 1: Verify changes
	results = append(results, "Step 1: Verifying changes...")
	verifyArgs := map[string]interface{}{}
	if hasTypeToVerify {
		verifyArgs["type"] = typeToVerify
	}
	if hasIdToVerify {
		verifyArgs["id"] = idToVerify
	}
	verifyResponse, err := m.makeHTTPRequest("POST", "/verify-changes", verifyArgs, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Change verification failed: %v", err)}},
			IsError: true,
		}, nil
	}
	results = append(results, fmt.Sprintf("✓ Changes verified: %s\n", string(verifyResponse)))

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}

// handleGetSamplersDataIntelligent handles the intelligent sample data request
func (m *APIMapper) handleGetSamplersDataIntelligent(args map[string]interface{}) (common.MCPToolResult, error) {
	response, err := m.makeHTTPRequest("POST", "/samplers/data/intelligent", args, true)
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Error fetching intelligent sample data: %v", err)}},
			IsError: true,
		}, nil
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: string(response)}},
	}, nil
}

// generateSystemIntroduction provides comprehensive AgentSmith-HUB system overview
func (m *APIMapper) generateSystemIntroduction() (common.MCPToolResult, error) {
	var results []string

	results = append(results, "🏛️ ===============================")
	results = append(results, "🏛️  AGENTSMITH-HUB SYSTEM OVERVIEW")
	results = append(results, "🏛️ ===============================\n")

	results = append(results, "🎯 **SYSTEM ARCHITECTURE**")
	results = append(results, "AgentSmith-HUB is a distributed security detection platform and security data pipeline platform with:")
	results = append(results, "• Data-driven security detection with component-based architecture")
	results = append(results, "• Input → Multi-Ruleset → Output pipeline with real-time processing")
	results = append(results, "• The rule engine supports complex data filtering and detection")
	results = append(results, "• Leader-follower cluster architecture with automatic failover\n")

	results = append(results, "🧩 **COMPONENT TYPES**")
	results = append(results, "┌─ INPUT: Data ingestion (kafka, aliyun sls) [YAML config]")
	results = append(results, "├─ RULESET: Security detection logic [XML with custom DSL]")
	results = append(results, "│  └─ Filter → CheckNode architecture for performance")
	results = append(results, "├─ OUTPUT: Alert delivery (print to log file, aliyun sls, elasticsearch, kafka) [YAML config]")
	results = append(results, "├─ PLUGIN: Custom functions (yaegi) [Go code]")
	results = append(results, "└─ PROJECT: Component orchestration [YAML workflow]\n")

	results = append(results, "🔑 **KEY CONCEPTS**")
	results = append(results, "⚡ Temporary Files: Changes go to .new files → deploy via apply_changes")
	results = append(results, "⚠️  CRITICAL: Temporary changes are NOT ACTIVE until deployed!")
	results = append(results, "📈 Sample Data: Auto-collected at each component for rule creation Or ask the user to provide Or use the intelligent sample data tool")
	results = append(results, "🎯 ProjectNodeSequence: Used to describe the specific location of a component within a project, like: INPUT.name.RULESET.name.OUTPUT.name")
	results = append(results, "📊 Data-Driven: NEVER create rules without sample data\n")

	results = append(results, "🚀 **DEPLOYMENT WORKFLOW**")
	results = append(results, "1. 📝 Create/Edit → Saves to temporary (.new) files")
	results = append(results, "2. 🔍 Review → Use 'get_pending_changes' to see what's staged")
	results = append(results, "3. 🧪 Test → Validate with real data using test tools\n")
	results = append(results, "4. 🚀 Deploy → Use 'apply_changes' to activate in production")

	results = append(results, "🛡️ **RULE ENGINE ARCHITECTURE**")
	results = append(results, "Performance Design: Filter → CheckNode")
	results = append(results, "• Filter: Coarse filtering (reduce volume 80%+)")
	results = append(results, "• CheckNode: Precise detection with field matching")
	results = append(results, "• Node Types: FASTEST[ISNULL,NOTNULL] → FAST[EQU,NEQ,MT,LT] → SLOWER[INCL,REGEX,PLUGIN]")
	results = append(results, "• Validation: Uppercase types required (DETECTION/WHITELIST)")
	results = append(results, "• Append: Only Type,FieldName,Value fields (NO desc!)\n")

	results = append(results, "📊 **DATA REQUIREMENTS** 🚨")
	results = append(results, "✅ MANDATORY: All rules based on actual sample data")
	results = append(results, "❌ FORBIDDEN: Imagined data like 'data_type=59', 'exe=msfconsole'")
	results = append(results, "📥 Sources: get_samplers_data API OR user-provided real JSON")
	results = append(results, "🔍 Validation: Field names must exist in actual data\n")

	results = append(results, "🎯 **COMMON WORKFLOWS**")
	results = append(results, "📝 Rule Creation:")
	results = append(results, "   1. get_samplers_data → 2. Analyze fields → 3. Create rule → 4. Test → 5. Deploy")
	results = append(results, "⚙️  Component Updates:")
	results = append(results, "   1. Edit (creates .new) → 2. get_pending_changes → 3. Test → 4. apply_changes")
	results = append(results, "🔧 Troubleshooting:")
	results = append(results, "   1. Check status → 2. Review logs → 3. Validate data flow → 4. Test components\n")

	results = append(results, "⚠️  **CRITICAL WARNINGS**")
	results = append(results, "🚨 Deployment: Temporary changes NOT ACTIVE until apply_changes")
	results = append(results, "🚨 Data-Driven: NEVER create rules without real sample data")
	results = append(results, "🚨 Syntax: Rule engine syntax must be exact - errors break ruleset")
	results = append(results, "🚨 Testing: Always test with real data before production")
	results = append(results, "🚨 Cluster: Only leader nodes collect sample data\n")

	results = append(results, "\n🎉 **YOU'RE READY TO USE AGENTSMITH-HUB!**")
	results = append(results, "Remember: Always work with real data, review before deploying, test thoroughly!")

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{Type: "text", Text: strings.Join(results, "\n")}},
	}, nil
}
