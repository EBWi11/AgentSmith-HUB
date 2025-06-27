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
		// === PUBLIC ENDPOINTS ===
		{
			Name:        "ping",
			Description: "Health check endpoint that returns 'pong' to verify server connectivity and basic status.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "token_check",
			Description: "Verify authentication token validity and retrieve current authentication status information.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_qps_data",
			Description: "Retrieve current QPS (Queries Per Second) metrics for all components. Returns real-time throughput data including per-component processing rates, message counts, and performance statistics. Supports filtering by project_id, component_id, node_id, and aggregation options.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id":     {Type: "string", Description: "Filter by specific project ID", Required: false},
				"node_id":        {Type: "string", Description: "Filter by specific node ID", Required: false},
				"component_id":   {Type: "string", Description: "Filter by specific component ID", Required: false},
				"component_type": {Type: "string", Description: "Filter by component type (input, output, ruleset)", Required: false},
				"aggregated":     {Type: "string", Description: "Return aggregated data (true/false)", Required: false},
			},
		},
		{
			Name:        "get_qps_stats",
			Description: "Get statistical analysis of QPS data including averages, trends, and performance metrics over time.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_hourly_messages",
			Description: "Retrieve hourly message processing statistics for time-based performance analysis. Returns message counts by hour with breakdown by component type and processing status.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id": {Type: "string", Description: "Filter by specific project ID", Required: false},
				"node_id":    {Type: "string", Description: "Filter by specific node ID", Required: false},
				"aggregated": {Type: "string", Description: "Return aggregated data (true/false)", Required: false},
				"by_node":    {Type: "string", Description: "Group results by node (true/false)", Required: false},
			},
		},
		{
			Name:        "get_daily_messages",
			Description: "Get daily message processing totals and trends for long-term analysis. Returns daily totals by project and component type with comparative statistics.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id": {Type: "string", Description: "Filter by specific project ID", Required: false},
				"node_id":    {Type: "string", Description: "Filter by specific node ID", Required: false},
				"aggregated": {Type: "string", Description: "Return aggregated data (true/false)", Required: false},
				"by_node":    {Type: "string", Description: "Group results by node (true/false)", Required: false},
			},
		},
		{
			Name:        "get_system_metrics",
			Description: "Retrieve current system performance metrics including CPU usage, memory consumption, goroutine count, and disk usage from the local node.",
			InputSchema: map[string]common.MCPToolArg{
				"since":   {Type: "string", Description: "Get metrics since specific timestamp (RFC3339 format)", Required: false},
				"current": {Type: "string", Description: "Return only current metrics (true/false)", Required: false},
			},
		},
		{
			Name:        "get_system_stats",
			Description: "Get statistical analysis of system performance metrics including averages, peaks, and trends over time.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_cluster_system_metrics",
			Description: "Retrieve system performance metrics from all nodes in the cluster for comprehensive cluster monitoring.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_cluster_system_stats",
			Description: "Get statistical analysis of cluster-wide system performance including node comparisons and performance distribution.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_cluster_status",
			Description: "Retrieve cluster status information including node health, leader election status, and connectivity information.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_cluster",
			Description: "Get detailed cluster configuration and membership information including node addresses and cluster settings.",
			InputSchema: map[string]common.MCPToolArg{},
		},

		// === PROJECT ENDPOINTS ===
		{
			Name:        "get_projects",
			Description: "Retrieve list of all projects with their status, configuration summary, and metadata. Returns project IDs, running status, and temporary file indicators.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_project",
			Description: "Get detailed configuration and status information for a specific project including YAML configuration and file paths.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to retrieve", Required: true},
			},
		},
		{
			Name:        "create_project",
			Description: "Create a new project with specified ID and YAML configuration. Creates temporary file that requires separate application step.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Unique project identifier", Required: true},
				"raw": {Type: "string", Description: "Project YAML configuration content", Required: true},
			},
		},
		{
			Name:        "update_project",
			Description: "Update existing project configuration with modified YAML content. Creates temporary file that requires application via apply_single_change.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Project identifier to update", Required: true},
				"raw": {Type: "string", Description: "Updated project YAML configuration", Required: true},
			},
		},
		{
			Name:        "delete_project",
			Description: "Delete specified project and all associated resources including configurations and temporary files.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to delete", Required: true},
			},
		},
		{
			Name:        "start_project",
			Description: "Start specified project and begin message processing. Applies pending changes automatically before starting.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id": {Type: "string", Description: "Project identifier to start", Required: true},
			},
		},
		{
			Name:        "stop_project",
			Description: "Stop specified project gracefully, completing current message processing before shutdown.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id": {Type: "string", Description: "Project identifier to stop", Required: true},
			},
		},
		{
			Name:        "restart_project",
			Description: "Restart specified project by stopping it gracefully and starting it again with latest configuration.",
			InputSchema: map[string]common.MCPToolArg{
				"project_id": {Type: "string", Description: "Project identifier to restart", Required: true},
			},
		},
		{
			Name:        "restart_all_projects",
			Description: "Restart all projects in the system sequentially with proper error handling and status reporting.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_project_error",
			Description: "Retrieve detailed error information for projects in error status including error messages and timestamps.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to get error details for", Required: true},
			},
		},
		{
			Name:        "get_project_inputs",
			Description: "Get list of input components and input nodes available for specified project, used for testing and validation.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to get input components for", Required: true},
			},
		},
		{
			Name:        "get_project_components",
			Description: "Retrieve component usage statistics for specified project including counts by component type and relationships.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to analyze components for", Required: true},
			},
		},
		{
			Name:        "get_project_component_sequences",
			Description: "Get component processing sequences and workflow paths for specified project showing message flow order.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to get component sequences for", Required: true},
			},
		},

		// === RULESET ENDPOINTS ===
		{
			Name:        "get_rulesets",
			Description: "Retrieve list of all rulesets with their metadata and temporary file indicators. Returns ruleset IDs and hasTemp flags.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_ruleset",
			Description: "Get detailed information for specific ruleset including complete XML configuration and file path information.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Ruleset identifier to retrieve", Required: true},
			},
		},
		{
			Name:        "create_ruleset",
			Description: "Create new ruleset with specified ID and XML configuration. Creates temporary file that requires separate application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Unique ruleset identifier", Required: true},
				"raw": {Type: "string", Description: "Ruleset XML configuration content", Required: true},
			},
		},
		{
			Name:        "update_ruleset",
			Description: "Update existing ruleset with modified XML configuration. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Ruleset identifier to update", Required: true},
				"raw": {Type: "string", Description: "Updated ruleset XML configuration", Required: true},
			},
		},
		{
			Name:        "delete_ruleset",
			Description: "Delete specified ruleset and all associated resources including configuration files and references.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Ruleset identifier to delete", Required: true},
			},
		},

		// === INPUT ENDPOINTS ===
		{
			Name:        "get_inputs",
			Description: "Retrieve list of all input components with their metadata and temporary file indicators. Returns input IDs and hasTemp flags.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_input",
			Description: "Get detailed information for specific input component including complete YAML configuration and file path.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Input component identifier to retrieve", Required: true},
			},
		},
		{
			Name:        "create_input",
			Description: "Create new input component with specified ID and YAML configuration. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Unique input identifier", Required: true},
				"raw": {Type: "string", Description: "Input YAML configuration content", Required: true},
			},
		},
		{
			Name:        "update_input",
			Description: "Update existing input component with modified YAML configuration. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Input identifier to update", Required: true},
				"raw": {Type: "string", Description: "Updated input YAML configuration", Required: true},
			},
		},
		{
			Name:        "delete_input",
			Description: "Delete specified input component and all associated resources including configuration files.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Input identifier to delete", Required: true},
			},
		},

		// === OUTPUT ENDPOINTS ===
		{
			Name:        "get_outputs",
			Description: "Retrieve list of all output components with their metadata, type information, and temporary file indicators.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_output",
			Description: "Get detailed information for specific output component including complete YAML configuration, type, and file path.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Output component identifier to retrieve", Required: true},
			},
		},
		{
			Name:        "create_output",
			Description: "Create new output component with specified ID and YAML configuration. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Unique output identifier", Required: true},
				"raw": {Type: "string", Description: "Output YAML configuration content", Required: true},
			},
		},
		{
			Name:        "update_output",
			Description: "Update existing output component with modified YAML configuration. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Output identifier to update", Required: true},
				"raw": {Type: "string", Description: "Updated output YAML configuration", Required: true},
			},
		},
		{
			Name:        "delete_output",
			Description: "Delete specified output component and all associated resources including configuration files.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Output identifier to delete", Required: true},
			},
		},

		// === PLUGIN ENDPOINTS ===
		{
			Name:        "get_plugins",
			Description: "Retrieve list of all plugins with their type information (local, yaegi, new), return types, and temporary file indicators.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_plugin",
			Description: "Get detailed information for specific plugin including source code, type information, and file path.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Plugin identifier to retrieve", Required: true},
			},
		},
		{
			Name:        "create_plugin",
			Description: "Create new plugin with specified ID and source code. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Unique plugin identifier", Required: true},
				"raw": {Type: "string", Description: "Plugin source code content", Required: true},
			},
		},
		{
			Name:        "update_plugin",
			Description: "Update existing plugin with modified source code. Creates temporary file that requires application.",
			InputSchema: map[string]common.MCPToolArg{
				"id":  {Type: "string", Description: "Plugin identifier to update", Required: true},
				"raw": {Type: "string", Description: "Updated plugin source code", Required: true},
			},
		},
		{
			Name:        "delete_plugin",
			Description: "Delete specified plugin and all associated resources including source code files.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Plugin identifier to delete", Required: true},
			},
		},
		{
			Name:        "get_available_plugins",
			Description: "Get list of available built-in plugins and plugin templates that can be used in configurations.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_plugin_parameters",
			Description: "Get parameter specifications and function signature information for specified plugin.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Plugin identifier to get parameter information for", Required: true},
			},
		},

		// === TESTING AND VERIFICATION ENDPOINTS ===
		{
			Name:        "verify_component",
			Description: "Validate component configuration syntax, connectivity, and compatibility. Returns detailed validation results with error messages and line numbers.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type (input, output, ruleset, plugin, project)", Required: true},
				"id":   {Type: "string", Description: "Component identifier to validate", Required: true},
			},
		},
		{
			Name:        "connect_check",
			Description: "Test connectivity and authentication for input/output components. Performs actual connection attempts and returns status.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type (input or output)", Required: true},
				"id":   {Type: "string", Description: "Component identifier to test connection for", Required: true},
			},
		},
		{
			Name:        "test_plugin",
			Description: "Execute plugin function with provided test data and return processing results with execution metrics.",
			InputSchema: map[string]common.MCPToolArg{
				"id":        {Type: "string", Description: "Plugin identifier to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},
		{
			Name:        "test_plugin_content",
			Description: "Test plugin source code directly without saving. Compiles and executes code with test data temporarily.",
			InputSchema: map[string]common.MCPToolArg{
				"content":   {Type: "string", Description: "Plugin source code to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},
		{
			Name:        "test_ruleset",
			Description: "Execute ruleset processing with sample data and return rule matching results with execution trace.",
			InputSchema: map[string]common.MCPToolArg{
				"id":        {Type: "string", Description: "Ruleset identifier to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},
		{
			Name:        "test_ruleset_content",
			Description: "Test ruleset XML configuration directly without saving. Parses XML and executes rules with test data temporarily.",
			InputSchema: map[string]common.MCPToolArg{
				"content":   {Type: "string", Description: "Ruleset XML configuration to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},
		{
			Name:        "test_output",
			Description: "Test output component delivery functionality with sample data. Attempts actual delivery and returns results.",
			InputSchema: map[string]common.MCPToolArg{
				"id":        {Type: "string", Description: "Output identifier to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to deliver", Required: true},
			},
		},
		{
			Name:        "test_project",
			Description: "Execute complete project workflow with test data. Processes data through entire pipeline and returns execution trace.",
			InputSchema: map[string]common.MCPToolArg{
				"id":        {Type: "string", Description: "Project identifier to test", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},
		{
			Name:        "test_project_content",
			Description: "Test project configuration directly without saving. Executes project workflow temporarily with provided configuration.",
			InputSchema: map[string]common.MCPToolArg{
				"inputNode": {Type: "string", Description: "Input node identifier to start workflow testing", Required: true},
				"test_data": {Type: "string", Description: "JSON formatted test data to process", Required: true},
			},
		},

		// === CLUSTER MANAGEMENT ===
		{
			Name:        "cluster_heartbeat",
			Description: "Send heartbeat signal to maintain cluster node health status and coordinate cluster membership.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Heartbeat data including node status and metrics", Required: true},
			},
		},
		{
			Name:        "component_sync",
			Description: "Synchronize component configurations across cluster nodes to ensure consistency.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Component synchronization data", Required: true},
			},
		},
		{
			Name:        "project_status_sync",
			Description: "Synchronize project status information across cluster nodes for consistent state management.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Project status synchronization data", Required: true},
			},
		},
		{
			Name:        "qps_sync",
			Description: "Synchronize QPS and performance metrics across cluster nodes for system-wide monitoring.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "QPS synchronization data", Required: true},
			},
		},
		{
			Name:        "get_config_root",
			Description: "Retrieve cluster leader's configuration root directory path for follower node synchronization.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "download_config",
			Description: "Download complete configuration package from cluster leader for node synchronization.",
			InputSchema: map[string]common.MCPToolArg{},
		},

		// === PENDING CHANGES MANAGEMENT ===
		{
			Name:        "get_pending_changes",
			Description: "Retrieve list of all pending configuration changes awaiting application to production components.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_enhanced_pending_changes",
			Description: "Retrieve comprehensive pending changes information with validation results and dependency analysis.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "apply_changes",
			Description: "Apply all pending configuration changes to production components with validation and rollback support.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "apply_changes_enhanced",
			Description: "Apply pending changes with enhanced transaction support and dependency resolution.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "apply_single_change",
			Description: "Apply specific individual pending change with focused validation and immediate feedback.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Change data including component type and identifier", Required: true},
			},
		},
		{
			Name:        "verify_changes",
			Description: "Validate all pending changes without applying them, providing comprehensive validation results.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "verify_change",
			Description: "Validate specific individual pending change with detailed analysis and compatibility checking.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type to validate", Required: true},
				"id":   {Type: "string", Description: "Component identifier to validate", Required: true},
			},
		},
		{
			Name:        "cancel_change",
			Description: "Cancel specific pending change and remove it from deployment queue with cleanup.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type of change to cancel", Required: true},
				"id":   {Type: "string", Description: "Component identifier of change to cancel", Required: true},
			},
		},
		{
			Name:        "cancel_all_changes",
			Description: "Cancel all pending changes and clean up the entire deployment queue with comprehensive cleanup.",
			InputSchema: map[string]common.MCPToolArg{},
		},

		// === TEMPORARY FILE MANAGEMENT ===
		{
			Name:        "create_temp_file",
			Description: "Create temporary configuration file for safe editing before applying changes to production.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type for temporary file", Required: true},
				"id":   {Type: "string", Description: "Component identifier", Required: true},
				"data": {Type: "string", Description: "Configuration data to write", Required: true},
			},
		},
		{
			Name:        "check_temp_file",
			Description: "Check status and existence of temporary configuration files for specific components.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type to check", Required: true},
				"id":   {Type: "string", Description: "Component identifier to check", Required: true},
			},
		},
		{
			Name:        "delete_temp_file",
			Description: "Delete temporary configuration file and clean up associated editing session data.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type for deletion", Required: true},
				"id":   {Type: "string", Description: "Component identifier for deletion", Required: true},
			},
		},

		// === SAMPLER ENDPOINTS ===
		{
			Name:        "get_samplers_data",
			Description: "Retrieve sample data and field analysis from system components for ruleset development.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_ruleset_fields",
			Description: "Get detailed field mapping and schema information for specific rulesets for development assistance.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Ruleset identifier to get field information for", Required: true},
			},
		},

		// === CANCEL UPGRADE ROUTES ===
		{
			Name:        "cancel_ruleset_upgrade",
			Description: "Cancel pending ruleset upgrade process and revert to previous stable version.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Ruleset identifier to cancel upgrade for", Required: true},
			},
		},
		{
			Name:        "cancel_input_upgrade",
			Description: "Cancel pending input component upgrade process and revert to previous stable configuration.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Input identifier to cancel upgrade for", Required: true},
			},
		},
		{
			Name:        "cancel_output_upgrade",
			Description: "Cancel pending output component upgrade process and revert to previous stable configuration.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Output identifier to cancel upgrade for", Required: true},
			},
		},
		{
			Name:        "cancel_project_upgrade",
			Description: "Cancel pending project upgrade process and revert to previous stable configuration.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Project identifier to cancel upgrade for", Required: true},
			},
		},
		{
			Name:        "cancel_plugin_upgrade",
			Description: "Cancel pending plugin upgrade process and revert to previous stable version.",
			InputSchema: map[string]common.MCPToolArg{
				"id": {Type: "string", Description: "Plugin identifier to cancel upgrade for", Required: true},
			},
		},

		// === COMPONENT USAGE ANALYSIS ===
		{
			Name:        "get_component_usage",
			Description: "Analyze component usage patterns and dependencies across projects for impact assessment.",
			InputSchema: map[string]common.MCPToolArg{
				"type": {Type: "string", Description: "Component type to analyze", Required: true},
				"id":   {Type: "string", Description: "Component identifier to analyze", Required: true},
			},
		},

		// === SEARCH ===
		{
			Name:        "search_components",
			Description: "Search through component configurations and content using flexible query patterns for discovery.",
			InputSchema: map[string]common.MCPToolArg{
				"query": {Type: "string", Description: "Search query string", Required: true},
			},
		},

		// === LOCAL CHANGES ===
		{
			Name:        "get_local_changes",
			Description: "Retrieve list of local configuration changes detected in filesystem that haven't been loaded.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "load_local_changes",
			Description: "Load all detected local configuration changes from filesystem into the system with validation.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "load_single_local_change",
			Description: "Load specific individual local configuration change from filesystem with validation.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Local change data to load", Required: true},
			},
		},

		// === METRICS SYNC ===
		{
			Name:        "metrics_sync",
			Description: "Synchronize performance metrics and monitoring data across cluster nodes for consistent reporting.",
			InputSchema: map[string]common.MCPToolArg{
				"data": {Type: "object", Description: "Metrics synchronization data", Required: true},
			},
		},

		// === ERROR LOGS ===
		{
			Name:        "get_error_logs",
			Description: "Retrieve system error logs and diagnostic information from local node for troubleshooting.",
			InputSchema: map[string]common.MCPToolArg{},
		},
		{
			Name:        "get_cluster_error_logs",
			Description: "Retrieve aggregated error logs from all cluster nodes for comprehensive system monitoring.",
			InputSchema: map[string]common.MCPToolArg{},
		},
	}
}

// CallAPITool calls the corresponding API endpoint for a given tool
func (m *APIMapper) CallAPITool(toolName string, args map[string]interface{}) (common.MCPToolResult, error) {
	// Map tool names to API endpoints and methods
	endpointMap := map[string]struct {
		method   string
		endpoint string
		auth     bool
	}{
		// Public endpoints
		"ping":                       {"GET", "/ping", false},
		"token_check":                {"GET", "/token-check", false},
		"get_qps_data":               {"GET", "/qps-data", false},
		"get_qps_stats":              {"GET", "/qps-stats", false},
		"get_hourly_messages":        {"GET", "/hourly-messages", false},
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
		"get_rulesets":   {"GET", "/rulesets", true},
		"get_ruleset":    {"GET", "/rulesets/%s", true},
		"create_ruleset": {"POST", "/rulesets", true},
		"update_ruleset": {"PUT", "/rulesets/%s", true},
		"delete_ruleset": {"DELETE", "/rulesets/%s", true},

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

		// Sampler endpoints
		"get_samplers_data":  {"GET", "/samplers/data", true},
		"get_ruleset_fields": {"GET", "/ruleset-fields/%s", true},

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
				key == "ruleset_name" || key == "input_name" || key == "output_name" || key == "plugin_name" {
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
					Format: "text",
					Text:   fmt.Sprintf("Error calling tool: %v", err),
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
				Format: "text", // MCP only supports "text" and "image" formats
				Text:   prettyResponseBody,
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
