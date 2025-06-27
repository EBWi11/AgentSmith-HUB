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
			Description: "Health check endpoint for AgentSmith-HUB server connectivity and basic status verification. Used by Dashboard to monitor system availability and by clients to test network connectivity before authentication. Returns server status, version info, and basic health metrics. No authentication required - ideal for automated monitoring, load balancer health checks, and connection troubleshooting. Response includes timestamp, uptime, and basic system health indicators.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "token_check",
			Description: "Verify authentication token validity and retrieve current authentication status. Frontend uses this during app initialization and token refresh cycles to ensure user remains authenticated. Returns token expiration info, user permissions, and session details. Essential for maintaining secure sessions - automatically called by frontend interceptors to handle token expiration gracefully. Use when implementing authentication flows or debugging access issues.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_qps_data",
			Description: "Retrieve current Queries Per Second (QPS) data for all components across the AgentSmith-HUB cluster. Dashboard uses this for real-time performance monitoring, displaying component throughput and identifying processing bottlenecks. Returns per-component QPS metrics, including input rates, output rates, and processing latencies. Supports filtering by project_id, component type, and time ranges. Critical for performance analysis, capacity planning, and troubleshooting high-load scenarios.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_qps_stats",
			Description: "Get aggregated QPS statistics and historical performance metrics across the entire AgentSmith-HUB system. Provides statistical analysis including averages, peaks, trends, and comparative data over time periods. Dashboard uses this for performance trending charts and system health indicators. Returns min/max/avg QPS values, percentile distributions, and growth trends. Essential for capacity planning, performance optimization, and identifying usage patterns.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_hourly_messages",
			Description: "Retrieve hourly message processing statistics for detailed time-based analysis of system throughput. Used by Dashboard and monitoring tools to create hourly performance charts and detect usage patterns. Returns message counts broken down by hour, component type, and processing status. Supports filtering by project, date range, and aggregation level. Valuable for understanding peak usage times, planning maintenance windows, and analyzing system load distribution throughout the day.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_daily_messages",
			Description: "Get daily message processing totals and trends for long-term system analysis and reporting. Dashboard displays these metrics in summary cards and trend charts to show system growth and daily throughput. Returns daily totals by project, component type, success/failure rates, and comparative data. Essential for business reporting, capacity planning, and identifying long-term usage trends. Supports aggregation by project_id and node_id for cluster-wide analysis.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_system_metrics",
			Description: "Retrieve current system performance metrics including CPU, memory, disk usage, and network statistics from the local node. Dashboard uses this for real-time system health monitoring and resource utilization tracking. Returns detailed metrics like CPU percentage, memory usage, disk I/O, network throughput, and process counts. Critical for system administration, performance monitoring, and identifying resource constraints that might affect processing performance.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_system_stats",
			Description: "Get aggregated system statistics and historical performance data for trend analysis and capacity planning. Provides statistical summaries including averages, peaks, and trends over configurable time periods. Used by Dashboard for system health indicators and performance trending. Returns statistical analysis of CPU, memory, and disk usage patterns. Essential for identifying performance degradation, planning hardware upgrades, and optimizing system configuration.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_cluster_system_metrics",
			Description: "Retrieve system performance metrics from all nodes in the AgentSmith-HUB cluster for comprehensive cluster monitoring. Dashboard displays cluster-wide system health, comparing performance across nodes and identifying potential issues. Returns per-node metrics including CPU, memory, disk usage, and network statistics. Supports node filtering and aggregation options. Critical for cluster administration, load balancing decisions, and identifying underperforming nodes.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_cluster_system_stats",
			Description: "Get statistical analysis of cluster-wide system performance including averages, trends, and comparative metrics across all nodes. Provides insights into cluster health, performance distribution, and resource utilization patterns. Dashboard uses this for cluster health overview and performance comparison charts. Returns statistical summaries, outlier detection, and performance rankings by node. Essential for cluster optimization, capacity planning, and identifying nodes requiring attention.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_cluster_status",
			Description: "Retrieve comprehensive cluster status information including node health, leader election status, and cluster topology. Dashboard displays cluster overview with node roles, connectivity status, and health indicators. Returns detailed information about each node including role (leader/follower), last heartbeat, active status, and version information. Critical for cluster administration, troubleshooting connectivity issues, and monitoring cluster stability and leadership changes.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_cluster",
			Description: "Get detailed cluster configuration and membership information including node addresses, ports, and cluster settings. Provides comprehensive cluster topology data for administration and monitoring tools. Returns cluster configuration, node list with detailed connection info, cluster-wide settings, and membership status. Used for cluster setup verification, network troubleshooting, and configuration management. Essential for understanding cluster architecture and connectivity requirements.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},

		// === PROJECT ENDPOINTS ===
		{
			Name:        "get_projects",
			Description: "Retrieve complete list of all projects in the AgentSmith-HUB system with their current status, configuration summary, and runtime information. Frontend uses this for project listing in sidebar, Dashboard overview, and project management interfaces. Returns project IDs, status (running/stopped/error), component counts, message throughput, and last activity timestamps. Includes temporary file indicators for projects with pending changes. Essential for project overview, status monitoring, and navigation.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_project",
			Description: "Get a specific project",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_project",
			Description: "Create a new project with specified configuration in AgentSmith-HUB system. Used by project creation forms and bulk import tools. Requires unique project ID and YAML configuration content. Validates configuration syntax, checks for component dependencies, and initializes project structure. Returns creation status, validation results, and any configuration warnings. Project will be created in 'stopped' status - use start_project to begin processing. Automatically creates necessary directory structure and configuration files.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Unique project identifier (e.g., 'new_api_project'). Must be alphanumeric with underscores, no spaces."},
					"content": map[string]interface{}{"type": "string", "description": "Complete project configuration in YAML format including inputs, outputs, rulesets, and processing flow definition."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "update_project",
			Description: "Update existing project configuration with new settings, component mappings, or processing flow changes. Used by project editor to save configuration changes. Creates temporary file for validation before applying changes. Requires project ID and updated YAML content. Validates new configuration, checks component dependencies, and maintains processing state. Returns update status and validation results. Note: Updates create pending changes that must be applied via apply_single_change for safety.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Existing project identifier to update."},
					"content": map[string]interface{}{"type": "string", "description": "Updated project configuration in YAML format with modified settings, components, or processing flow."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "delete_project",
			Description: "Delete specified project and all associated resources including configurations, temporary files, and runtime data. Used by project management interface with confirmation dialogs. Requires project ID parameter. Automatically stops project if running, removes all associated files, and cleans up cluster-wide references. Returns deletion status and cleanup summary. WARNING: This operation is irreversible and will permanently remove all project data, configurations, and historical statistics.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to delete permanently."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "start_project",
			Description: "Start specified project and begin message processing according to its configuration. Frontend uses this for project control buttons and automated startup sequences. Applies any pending configuration changes automatically before starting. Requires project_id parameter. Initializes all input/output connections, loads rulesets, and begins message flow. Returns startup status, component initialization results, and any startup errors. Project status changes to 'running' on success or 'error' if startup fails.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{"type": "string", "description": "Project identifier to start. Must be in 'stopped' status and have valid configuration."},
				},
				"required": []string{"project_id"},
			},
		},
		{
			Name:        "stop_project",
			Description: "Stop specified project gracefully, completing current message processing before shutdown. Used by project control interface and maintenance procedures. Applies pending changes before stopping to ensure latest configuration is saved. Requires project_id parameter. Gracefully closes all connections, completes in-flight messages, and updates project status. Returns stop status and final processing statistics. Project can be restarted later with same or updated configuration.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{"type": "string", "description": "Project identifier to stop. Must be in 'running' status."},
				},
				"required": []string{"project_id"},
			},
		},
		{
			Name:        "restart_project",
			Description: "Restart specified project by stopping it gracefully and starting it again with latest configuration. Frontend uses this for applying configuration changes and recovering from errors. Automatically applies any pending changes before restart. Combines stop and start operations with proper error handling. Returns restart status, configuration reload results, and startup verification. More reliable than separate stop/start calls as it handles timing and state transitions automatically.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{"type": "string", "description": "Project identifier to restart. Can be in any status - will be stopped first if running."},
				},
				"required": []string{"project_id"},
			},
		},
		{
			Name:        "restart_all_projects",
			Description: "Restart all projects in the system sequentially with proper dependency handling and error recovery. Used for system-wide configuration updates, maintenance procedures, and cluster synchronization. Processes projects in dependency order, handles failures gracefully, and provides comprehensive status reporting. Returns detailed restart results for each project including timing, errors, and final status. Critical for maintaining system consistency during updates or after cluster changes.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_project_error",
			Description: "Retrieve detailed error information for projects in error status including stack traces, configuration issues, and component failure details. Frontend displays this in project status indicators and error debugging interfaces. Requires project ID parameter. Returns comprehensive error details including error timestamps, affected components, configuration validation failures, and suggested remediation steps. Essential for troubleshooting project issues and identifying root causes of failures.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to get error details for. Project should be in 'error' status."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_project_inputs",
			Description: "Get list of input components and input nodes available for specified project, used for testing interfaces and project configuration validation. Frontend uses this to populate input selection dropdowns in test forms and workflow designers. Returns available input components with their types, connection status, and configuration details. Essential for project testing, debugging message flows, and validating input connectivity.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to get input components for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_project_components",
			Description: "Retrieve comprehensive component usage statistics for specified project including counts of inputs, outputs, rulesets, and plugins used. Dashboard uses this for project overview cards and component relationship visualization. Returns detailed component counts by type, component health status, dependency relationships, and usage statistics. Valuable for project analysis, dependency tracking, and identifying unused or problematic components.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to analyze components for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_project_component_sequences",
			Description: "Get detailed component processing sequences and workflow paths for specified project showing how messages flow through the system. Used by workflow visualization tools and debugging interfaces to understand message processing paths. Returns component execution order, data flow paths, branching logic, and processing sequences. Critical for understanding project architecture, optimizing processing flows, and troubleshooting message routing issues.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to get component sequences for."},
				},
				"required": []string{"id"},
			},
		},

		// === RULESET ENDPOINTS ===
		{
			Name:        "get_rulesets",
			Description: "Retrieve complete list of all rulesets in the system with their status, usage information, and temporary file indicators. Sidebar component uses this for ruleset navigation and management interfaces. Returns ruleset IDs, descriptions, associated projects, rule counts, and modification timestamps. Includes indicators for rulesets with pending changes (.new files). Essential for ruleset management, dependency tracking, and identifying unused or problematic rule configurations.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_ruleset",
			Description: "Get detailed information for specific ruleset including complete XML configuration, rule definitions, field mappings, and usage statistics. Used by ruleset editor, testing interfaces, and debugging tools. Requires ruleset ID parameter. Returns full XML content, parsed rule structure, field requirements, associated projects, and performance metrics. Critical for ruleset editing, validation, and understanding rule logic and data processing requirements.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Unique ruleset identifier (e.g., 'security_rules', 'data_validation'). Must match existing ruleset name."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_ruleset",
			Description: "Create new ruleset with specified XML configuration and rule definitions. Used by ruleset creation forms and import tools. Requires unique ruleset ID and XML content. Validates XML syntax, rule logic, and field requirements. Checks for rule conflicts and validates against schema. Returns creation status, validation results, and rule parsing summary. Created ruleset can be immediately used in project configurations and testing interfaces.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Unique ruleset identifier. Must be alphanumeric with underscores, no spaces or special characters."},
					"content": map[string]interface{}{"type": "string", "description": "Complete ruleset configuration in XML format with rule definitions, conditions, actions, and field mappings."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "update_ruleset",
			Description: "Update existing ruleset with modified XML configuration, rule changes, or field mapping updates. Ruleset editor uses this to save changes with validation. Creates temporary file for safety before applying changes. Validates XML syntax, rule logic consistency, and field compatibility. Returns update status, validation results, and change summary. Updates create pending changes that require explicit application for production use.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Existing ruleset identifier to update."},
					"content": map[string]interface{}{"type": "string", "description": "Updated ruleset XML configuration with modified rules, conditions, or field mappings."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "delete_ruleset",
			Description: "Delete specified ruleset and all associated resources including rule files and usage references. Used by ruleset management interface with dependency checking and confirmation. Checks for project dependencies before deletion and warns about impact. Removes ruleset files, cleans up references, and updates dependent projects. Returns deletion status and dependency impact summary. WARNING: Deleting active rulesets may cause project failures.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Ruleset identifier to delete. System will check for dependencies in active projects."},
				},
				"required": []string{"id"},
			},
		},

		// === INPUT ENDPOINTS ===
		{
			Name:        "get_inputs",
			Description: "Retrieve complete list of all input components in the system with their configuration status, connection health, and usage information. Sidebar uses this for input component navigation and management interfaces. Returns input IDs, types (kafka, api, file, etc.), connection status, associated projects, and temporary file indicators. Includes health status, throughput metrics, and last activity timestamps. Essential for input management, troubleshooting connectivity issues, and monitoring data ingestion performance.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_input",
			Description: "Get detailed information for specific input component including complete YAML configuration, connection parameters, health status, and performance metrics. Used by input editor, testing interfaces, and connection debugging tools. Requires input ID parameter. Returns full configuration content, connection details, authentication settings, message format specifications, and throughput statistics. Critical for input configuration editing, connection troubleshooting, and performance optimization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Unique input component identifier (e.g., 'kafka_input', 'api_endpoint_1'). Must match existing input name."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_input",
			Description: "Create new input component with specified configuration for data ingestion from external sources. Used by input creation forms and automated deployment tools. Requires unique input ID and YAML configuration. Validates connection parameters, authentication settings, and message format specifications. Tests connectivity during creation process. Returns creation status, validation results, and connection test outcomes. Created input can be immediately used in project configurations.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Unique input identifier. Must be alphanumeric with underscores, no spaces."},
					"content": map[string]interface{}{"type": "string", "description": "Complete input configuration in YAML format including connection details, authentication, message format, and processing settings."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "update_input",
			Description: "Update existing input component with modified configuration, connection parameters, or processing settings. Input editor uses this to save configuration changes with validation and connection testing. Creates temporary file for safety before applying changes. Validates new configuration, tests connectivity, and checks compatibility with existing projects. Returns update status, validation results, and connection test outcomes. Updates create pending changes requiring explicit application.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Existing input identifier to update."},
					"content": map[string]interface{}{"type": "string", "description": "Updated input configuration in YAML format with modified connection details, authentication, or processing settings."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "delete_input",
			Description: "Delete specified input component and all associated resources including configuration files and connection references. Used by input management interface with dependency checking and confirmation. Checks for project dependencies before deletion and warns about impact on active projects. Removes input files, closes connections, and updates dependent projects. Returns deletion status and dependency impact summary. WARNING: Deleting active inputs may cause project failures and data loss.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Input identifier to delete. System will check for dependencies in active projects."},
				},
				"required": []string{"id"},
			},
		},

		// === OUTPUT ENDPOINTS ===
		{
			Name:        "get_outputs",
			Description: "Retrieve complete list of all output components in the system with their configuration status, connection health, and delivery performance metrics. Sidebar uses this for output component navigation and management. Returns output IDs, types (elasticsearch, kafka, webhook, etc.), connection status, associated projects, and temporary file indicators. Includes delivery success rates, throughput metrics, and error statistics. Essential for output management, delivery monitoring, and performance optimization.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_output",
			Description: "Get detailed information for specific output component including complete YAML configuration, connection parameters, delivery settings, and performance statistics. Used by output editor, testing interfaces, and delivery monitoring tools. Requires output ID parameter. Returns full configuration content, connection details, authentication settings, delivery format specifications, retry policies, and success/failure metrics. Critical for output configuration editing and delivery troubleshooting.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Unique output component identifier (e.g., 'es_output', 'webhook_alerts'). Must match existing output name."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_output",
			Description: "Create new output component with specified configuration for data delivery to external systems. Used by output creation forms and deployment automation. Requires unique output ID and YAML configuration. Validates connection parameters, authentication credentials, and delivery format settings. Tests connectivity and delivery capability during creation. Returns creation status, validation results, and delivery test outcomes. Created output can be immediately used in project configurations.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Unique output identifier. Must be alphanumeric with underscores, no spaces."},
					"content": map[string]interface{}{"type": "string", "description": "Complete output configuration in YAML format including connection details, authentication, delivery format, retry policies, and performance settings."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "update_output",
			Description: "Update existing output component with modified configuration, connection parameters, or delivery settings. Output editor uses this to save configuration changes with validation and delivery testing. Creates temporary file for safety before applying changes. Validates new configuration, tests connectivity and delivery capability, and checks compatibility with existing projects. Returns update status, validation results, and delivery test outcomes. Updates create pending changes requiring explicit application.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Existing output identifier to update."},
					"content": map[string]interface{}{"type": "string", "description": "Updated output configuration in YAML format with modified connection details, authentication, delivery settings, or retry policies."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "delete_output",
			Description: "Delete specified output component and all associated resources including configuration files and delivery connections. Used by output management interface with dependency checking and confirmation. Checks for project dependencies before deletion and warns about impact on active delivery pipelines. Removes output files, closes connections, and updates dependent projects. Returns deletion status and dependency impact summary. WARNING: Deleting active outputs may cause project failures and data delivery interruption.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Output identifier to delete. System will check for dependencies in active projects."},
				},
				"required": []string{"id"},
			},
		},

		// === PLUGIN ENDPOINTS ===
		{
			Name:        "get_plugins",
			Description: "Retrieve complete list of all plugin components in the system with their implementation status, runtime information, and usage statistics. Sidebar uses this for plugin navigation and management interfaces. Returns plugin names, types (data processor, formatter, validator, etc.), implementation language, associated projects, and temporary file indicators. Includes execution performance metrics, error rates, and dependency information. Essential for plugin management, performance monitoring, and identifying unused or problematic plugins.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_plugin",
			Description: "Get detailed information for specific plugin including complete source code, function signatures, runtime performance, and usage statistics. Used by plugin editor, testing interfaces, and debugging tools. Requires plugin ID parameter. Returns full source code content, function documentation, parameter specifications, return value descriptions, execution metrics, and associated projects. Critical for plugin development, debugging, and performance optimization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Unique plugin identifier (e.g., 'data_transformer', 'ip_validator'). Must match existing plugin name."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_plugin",
			Description: "Create new plugin with specified source code and function implementation for custom data processing logic. Used by plugin development interface and code import tools. Requires unique plugin ID and source code content. Validates syntax, compiles code, and tests basic functionality. Checks for security vulnerabilities and performance issues. Returns creation status, compilation results, and initial test outcomes. Created plugin can be immediately used in project configurations and tested with sample data.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Unique plugin identifier. Must be alphanumeric with underscores, no spaces."},
					"content": map[string]interface{}{"type": "string", "description": "Complete plugin source code with function implementation, parameter definitions, and documentation comments."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "update_plugin",
			Description: "Update existing plugin with modified source code, function logic, or parameter definitions. Plugin editor uses this to save code changes with compilation and testing. Creates temporary file for safety before applying changes. Validates syntax, compiles updated code, and tests functionality. Checks for breaking changes that might affect existing projects. Returns update status, compilation results, and compatibility analysis. Updates create pending changes requiring explicit application.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Existing plugin identifier to update."},
					"content": map[string]interface{}{"type": "string", "description": "Updated plugin source code with modified function implementation or parameter definitions."},
				},
				"required": []string{"id", "content"},
			},
		},
		{
			Name:        "delete_plugin",
			Description: "Delete specified plugin and all associated resources including source code files and runtime references. Used by plugin management interface with dependency checking and confirmation. Checks for project dependencies before deletion and warns about impact on active processing pipelines. Removes plugin files, clears runtime cache, and updates dependent projects. Returns deletion status and dependency impact summary. WARNING: Deleting active plugins may cause project failures and processing errors.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Plugin identifier to delete. System will check for dependencies in active projects."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_available_plugins",
			Description: "Get list of available plugin templates and built-in plugins that can be used in project configurations. Used by plugin selection interfaces and project configuration tools. Returns plugin templates with descriptions, parameter specifications, usage examples, and compatibility information. Includes both system-provided plugins and user-created templates. Essential for plugin discovery, project configuration assistance, and understanding available data processing capabilities.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_plugin_parameters",
			Description: "Get detailed parameter specifications and function signature information for specified plugin. Used by testing interfaces, project configuration tools, and code completion features. Returns parameter names, types, descriptions, validation rules, default values, and usage examples. Includes function return value specifications and error handling information. Critical for plugin integration, automated testing, and development assistance.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Plugin identifier to get parameter information for."},
				},
				"required": []string{"id"},
			},
		},

		// === TESTING AND VERIFICATION ENDPOINTS ===
		{
			Name:        "verify_component",
			Description: "Validate component configuration syntax, connectivity, and compatibility with system requirements. Used by component editors and validation interfaces to ensure configurations are correct before deployment. Requires component type and ID parameters. Performs comprehensive validation including syntax checking, connection testing, dependency verification, and compatibility analysis. Returns detailed validation results with specific error messages, warnings, and remediation suggestions. Essential for preventing configuration errors and ensuring reliable system operation.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type to validate (inputs, outputs, rulesets, plugins, projects)."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier to validate."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "connect_check",
			Description: "Test connectivity and authentication for input/output components to verify external system integration. Used by connection testing interfaces and troubleshooting tools. Requires component type and ID parameters. Performs actual connection attempts, authentication verification, and basic communication tests. Returns connection status, response times, authentication results, and any connectivity issues. Critical for troubleshooting connectivity problems, validating credentials, and ensuring external system availability.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type (input or output) to test connectivity for."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier to test connection for."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "test_plugin",
			Description: "Execute plugin function with provided test data to validate plugin logic and performance. Frontend test interfaces use this to verify plugin implementations before deployment. Requires plugin ID and test data in JSON format. Executes plugin function, measures performance, and returns processing results with execution metrics. Returns function output, execution time, memory usage, and any runtime errors. Essential for plugin development, debugging function logic, and performance optimization. Supports complex data structures and validates return value formatting.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "description": "Plugin identifier to test with sample data."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through the plugin function."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "test_plugin_content",
			Description: "Test plugin source code directly without saving it as a component, ideal for development and iteration. Used by plugin development interface for rapid testing during code writing. Requires plugin source code and test data. Compiles code temporarily, executes with test data, and returns results without persisting changes. Returns function output, compilation status, execution metrics, and error details. Perfect for testing code changes before committing, validating new plugin logic, and debugging compilation issues.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content":   map[string]interface{}{"type": "string", "description": "Complete plugin source code to test temporarily."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through the plugin function."},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "test_ruleset",
			Description: "Execute ruleset processing with sample data to validate rule logic, field mappings, and conditional processing. Frontend testing interfaces use this to verify ruleset behavior before deployment. Requires ruleset ID and test data in JSON format. Processes data through all rules, evaluates conditions, and returns matching results with detailed rule execution trace. Returns matched rules, field transformations, processing statistics, and rule evaluation details. Critical for ruleset validation, debugging rule logic, and performance optimization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "description": "Ruleset identifier to test with sample data."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through ruleset rules and conditions."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "test_ruleset_content",
			Description: "Test ruleset XML configuration directly without saving it as a component, perfect for ruleset development and debugging. Used by ruleset development interface for iterative testing during rule creation. Requires ruleset XML content and test data. Parses XML temporarily, executes rules with test data, and returns results without persisting changes. Returns rule matching results, field processing details, validation errors, and performance metrics. Essential for validating rule syntax, testing conditional logic, and debugging field mappings.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content":   map[string]interface{}{"type": "string", "description": "Complete ruleset XML configuration to test temporarily."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through ruleset rules and conditions."},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "test_output",
			Description: "Test output component delivery functionality with sample data to validate connection, formatting, and delivery success. Frontend test interfaces use this to verify output configurations before deployment. Requires output ID and test data in JSON format. Attempts actual delivery to configured destination, validates formatting, and tests connection reliability. Returns delivery status, formatting results, response times, and any delivery errors. Critical for output validation, delivery troubleshooting, and performance testing.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "description": "Output identifier to test delivery functionality."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to deliver through the output component."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "test_project",
			Description: "Execute complete project workflow with test data to validate end-to-end processing pipeline including inputs, rulesets, plugins, and outputs. Frontend project testing interface uses this for comprehensive project validation. Requires project ID, input node specification, and test data. Processes data through entire project pipeline, tracking each component's execution and data transformations. Returns complete processing results, component execution trace, performance metrics, and output results. Essential for project integration testing, workflow validation, and performance analysis.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "description": "Project identifier to test complete workflow."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through the entire project pipeline."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "test_project_content",
			Description: "Test project configuration directly without saving it as a component, ideal for project development and workflow design validation. Used by project development interface for testing configuration changes before deployment. Requires project YAML content, input node specification, and test data. Executes project workflow temporarily with provided configuration and returns complete processing results. Returns component execution trace, data transformations, output results, and configuration validation details. Perfect for validating project architecture, testing workflow changes, and debugging processing issues.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"inputNode": map[string]interface{}{"type": "string", "description": "Input node identifier to start project workflow testing."},
					"test_data": map[string]interface{}{"type": "string", "description": "JSON formatted test data to process through the project workflow."},
				},
				"required": []string{"inputNode"},
			},
		},

		// === CLUSTER MANAGEMENT ===
		{
			Name:        "cluster_heartbeat",
			Description: "Send heartbeat signal to maintain cluster node health status and synchronize cluster membership. Used by cluster management systems for node health monitoring and leader election maintenance. Includes node status information, resource utilization data, and cluster coordination data. Returns heartbeat acknowledgment and cluster status updates. Critical for cluster stability, automatic failover, and maintaining distributed system consistency. Helps identify failed nodes and triggers cluster reconfiguration when necessary.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Heartbeat data including node status, resource metrics, and cluster coordination information."},
				},
			},
		},
		{
			Name:        "component_sync",
			Description: "Synchronize component configurations across cluster nodes to ensure consistency and prevent configuration drift. Used by cluster management systems during configuration updates and node synchronization. Propagates component changes, validates configuration consistency, and maintains cluster-wide component state. Returns synchronization status and any conflicts detected. Essential for maintaining configuration consistency, preventing split-brain scenarios, and ensuring all nodes have identical component definitions.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Component synchronization data including configurations, checksums, and version information."},
				},
			},
		},
		{
			Name:        "project_status_sync",
			Description: "Synchronize project running status and state information across cluster nodes for consistent project management. Used by cluster management systems to coordinate project lifecycle and status reporting. Propagates project status changes, handles status conflicts, and maintains cluster-wide project state consistency. Returns synchronization results and status reconciliation data. Critical for coordinated project management, accurate status reporting, and preventing inconsistent project states across nodes.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Project status synchronization data including running states, timestamps, and status changes."},
				},
			},
		},
		{
			Name:        "qps_sync",
			Description: "Synchronize QPS (Queries Per Second) and performance metrics across cluster nodes for accurate system-wide performance monitoring. Used by monitoring systems to aggregate performance data and generate cluster-wide statistics. Collects and distributes QPS data, performance metrics, and throughput statistics. Returns aggregated performance data and synchronization status. Essential for cluster performance monitoring, capacity planning, and identifying performance bottlenecks across the distributed system.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "QPS synchronization data including performance metrics, throughput statistics, and timing information."},
				},
			},
		},
		{
			Name:        "get_config_root",
			Description: "Retrieve cluster leader's configuration root directory path and configuration management settings. Used by follower nodes for configuration synchronization and by management tools for configuration discovery. Returns leader's configuration path, access permissions, and synchronization settings. Critical for cluster configuration management, automated synchronization, and ensuring follower nodes can access leader configurations for consistency maintenance.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "download_config",
			Description: "Download complete configuration package from cluster leader for node synchronization and configuration replication. Used by follower nodes during initial setup and periodic synchronization. Packages all configuration files, validates integrity, and provides secure configuration transfer. Returns configuration archive and verification data. Essential for cluster setup, configuration backup, and maintaining configuration consistency across all cluster nodes.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},

		// === PENDING CHANGES MANAGEMENT ===
		{
			Name:        "get_pending_changes",
			Description: "Retrieve list of all pending configuration changes awaiting application to production components. Frontend pending changes interface uses this to display changes requiring approval and deployment. Returns change details, component types, modification summaries, and creation timestamps. Includes validation status and impact analysis for each pending change. Essential for change management workflows, configuration review processes, and coordinated deployment of component updates.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_enhanced_pending_changes",
			Description: "Retrieve comprehensive pending changes information with enhanced details including validation results, dependency analysis, and impact assessment. Frontend uses this for detailed change review and approval workflows. Returns detailed change analysis, validation status, dependency trees, and risk assessment data. Includes component usage analysis and potential impact on running projects. Critical for enterprise change management, risk assessment, and informed deployment decisions.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "apply_changes",
			Description: "Apply all pending configuration changes to production components with validation and rollback capabilities. Frontend uses this for batch deployment of approved changes. Validates each change, applies configurations sequentially, and handles conflicts or failures gracefully. Returns detailed application results, success/failure status for each change, and rollback information if needed. Essential for coordinated deployment, change management workflows, and maintaining system stability during updates.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "apply_changes_enhanced",
			Description: "Apply pending changes with enhanced transaction support, dependency resolution, and advanced rollback capabilities. Used for enterprise-grade change management with complex dependency handling. Provides atomic transaction support, intelligent dependency ordering, and comprehensive rollback mechanisms. Returns detailed transaction results, dependency resolution logs, and complete rollback instructions. Critical for complex deployments, enterprise change management, and maintaining system consistency during large-scale updates.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "apply_single_change",
			Description: "Apply specific individual pending change with focused validation and immediate feedback. Frontend uses this for selective change deployment and testing individual components. Validates single change, applies configuration, and provides immediate results with detailed success/failure information. Returns change application status, configuration validation results, and any integration issues. Perfect for incremental deployment, individual component testing, and selective change management.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Specific change data including component type, identifier, and configuration details to apply."},
				},
			},
		},
		{
			Name:        "verify_changes",
			Description: "Validate all pending changes without applying them to production, providing comprehensive validation and impact analysis. Used by change review processes and automated validation pipelines. Performs syntax validation, dependency checking, compatibility analysis, and impact assessment. Returns detailed validation results, potential issues, and recommendations. Essential for change approval workflows, risk assessment, and ensuring change quality before deployment.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "verify_change",
			Description: "Validate specific individual pending change with detailed analysis and compatibility checking. Frontend change review interface uses this for individual change assessment. Performs comprehensive validation including syntax checking, dependency analysis, and impact assessment. Returns detailed validation results, compatibility analysis, and potential issues or conflicts. Critical for change review processes, quality assurance, and individual change validation before deployment.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type of the change to validate (inputs, outputs, rulesets, plugins, projects)."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier of the specific change to validate."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "cancel_change",
			Description: "Cancel specific pending change and remove it from the deployment queue with cleanup of associated temporary files. Frontend change management interface uses this for selective change cancellation. Removes pending change, cleans up temporary files, and updates change tracking. Returns cancellation status and cleanup results. Essential for change management workflows, correcting mistakes, and maintaining clean change queues.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type of the change to cancel."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier of the specific change to cancel."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "cancel_all_changes",
			Description: "Cancel all pending changes and clean up the entire deployment queue with comprehensive cleanup of temporary files and change tracking. Frontend uses this for bulk change cancellation and system reset. Removes all pending changes, performs comprehensive cleanup, and resets change management state. Returns detailed cleanup results and system state reset confirmation. Critical for emergency change management, system reset scenarios, and clearing problematic change queues.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},

		// === TEMPORARY FILE MANAGEMENT ===
		{
			Name:        "create_temp_file",
			Description: "Create temporary configuration file for safe editing and testing before applying changes to production components. Frontend editors use this for creating draft configurations and testing changes. Creates secure temporary file, tracks editing session, and maintains version control. Returns temporary file status and editing session information. Essential for safe configuration editing, change tracking, and preventing accidental production modifications during development.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type for temporary file creation (inputs, outputs, rulesets, plugins, projects)."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier for temporary file creation."},
					"data": map[string]interface{}{"type": "string", "description": "Configuration data to write to temporary file."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "check_temp_file",
			Description: "Check status and existence of temporary configuration files for specific components. Frontend interfaces use this to determine if components have unsaved changes or pending modifications. Verifies temporary file existence, checks modification timestamps, and validates file integrity. Returns temporary file status, modification information, and content validation results. Critical for change tracking, unsaved work detection, and maintaining editing session consistency.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type to check for temporary files."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier to check for temporary files."},
				},
				"required": []string{"type", "id"},
			},
		},
		{
			Name:        "delete_temp_file",
			Description: "Delete temporary configuration file and clean up associated editing session data. Frontend interfaces use this for canceling edits and cleaning up temporary changes. Removes temporary file, clears editing session data, and updates change tracking. Returns deletion status and cleanup confirmation. Essential for canceling unsaved changes, cleaning up editing sessions, and maintaining clean temporary file storage.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type for temporary file deletion."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier for temporary file deletion."},
				},
				"required": []string{"type", "id"},
			},
		},

		// === SAMPLER ENDPOINTS ===
		{
			Name:        "get_samplers_data",
			Description: "Retrieve sample data and field analysis from system components for ruleset development and data structure understanding. Frontend ruleset development interfaces use this to understand data patterns and field structures for rule creation. Returns sample messages, field distributions, data type analysis, and value patterns from recent system activity. Includes statistical analysis of field usage, common values, and data structure variations. Essential for ruleset development, field mapping creation, and understanding data patterns for effective rule design.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_ruleset_fields",
			Description: "Get detailed field mapping and schema information for specific rulesets including field types, usage patterns, and processing statistics. Frontend ruleset editor uses this for field autocomplete, validation, and schema assistance. Returns field names, data types, usage frequency, sample values, and processing statistics. Includes field relationship mapping and validation rules. Critical for ruleset development, field validation, and understanding data schema requirements for rule creation and debugging.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Ruleset identifier to get field mapping and schema information for."},
				},
				"required": []string{"id"},
			},
		},

		// === CANCEL UPGRADE ROUTES ===
		{
			Name:        "cancel_ruleset_upgrade",
			Description: "Cancel pending ruleset upgrade process and revert to previous stable version with complete rollback of changes. Frontend upgrade management interface uses this for aborting problematic upgrades. Stops upgrade process, reverts configuration changes, and restores previous working version. Returns cancellation status, rollback results, and system state restoration details. Critical for upgrade management, emergency rollback procedures, and maintaining system stability during failed upgrade attempts.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Ruleset identifier to cancel upgrade process for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "cancel_input_upgrade",
			Description: "Cancel pending input component upgrade process and revert to previous stable configuration with connection restoration. Frontend upgrade management uses this for aborting input component upgrades. Stops upgrade process, restores previous configuration, and re-establishes input connections. Returns cancellation status, connection restoration results, and configuration rollback details. Essential for input management, connection stability, and recovering from failed input component upgrades.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Input component identifier to cancel upgrade process for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "cancel_output_upgrade",
			Description: "Cancel pending output component upgrade process and revert to previous stable configuration with delivery restoration. Frontend upgrade management uses this for aborting output component upgrades. Stops upgrade process, restores previous configuration, and re-establishes output delivery connections. Returns cancellation status, delivery restoration results, and configuration rollback details. Critical for output management, delivery continuity, and recovering from failed output component upgrades.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Output component identifier to cancel upgrade process for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "cancel_project_upgrade",
			Description: "Cancel pending project upgrade process and revert to previous stable configuration with complete project state restoration. Frontend project management uses this for aborting project upgrades. Stops upgrade process, restores previous project configuration, and maintains project running state. Returns cancellation status, project state restoration results, and configuration rollback details. Essential for project management, service continuity, and recovering from failed project upgrades.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Project identifier to cancel upgrade process for."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "cancel_plugin_upgrade",
			Description: "Cancel pending plugin upgrade process and revert to previous stable version with runtime restoration. Frontend plugin management uses this for aborting plugin upgrades. Stops upgrade process, restores previous plugin version, and re-initializes plugin runtime. Returns cancellation status, runtime restoration results, and version rollback details. Critical for plugin management, processing continuity, and recovering from failed plugin upgrades.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Plugin identifier to cancel upgrade process for."},
				},
				"required": []string{"id"},
			},
		},

		// === COMPONENT USAGE ANALYSIS ===
		{
			Name:        "get_component_usage",
			Description: "Analyze component usage patterns and dependencies across projects to understand component utilization and impact assessment. Frontend component management interface uses this for dependency tracking and impact analysis before component modifications. Returns detailed usage statistics, dependent projects list, usage patterns, and impact assessment data. Includes performance metrics, dependency graphs, and usage trends. Essential for component lifecycle management, impact analysis, and safe component modifications or deletions.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Component type to analyze usage for (inputs, outputs, rulesets, plugins, projects)."},
					"id":   map[string]interface{}{"type": "string", "description": "Component identifier to analyze usage patterns and dependencies for."},
				},
				"required": []string{"type", "id"},
			},
		},

		// === SEARCH ===
		{
			Name:        "search_components",
			Description: "Search through component configurations, content, and metadata using flexible query patterns for component discovery and content analysis. Frontend global search interface uses this for finding components, configurations, and specific content patterns. Performs full-text search across component configurations, names, descriptions, and content. Returns matching components with relevance scoring, content snippets, and match context. Essential for component discovery, configuration debugging, and finding specific implementation patterns across the system.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "Search query string for finding components, configurations, or content patterns across the system."},
				},
				"required": []string{"query"},
			},
		},

		// === LOCAL CHANGES ===
		{
			Name:        "get_local_changes",
			Description: "Retrieve list of local configuration changes detected in filesystem that haven't been loaded into the system. Frontend local changes interface uses this to display available local changes for import. Scans local filesystem, detects configuration changes, and analyzes modification patterns. Returns list of changed files, modification details, and change analysis. Essential for development workflows, configuration import, and synchronizing local development changes with the running system.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "load_local_changes",
			Description: "Load all detected local configuration changes from filesystem into the system with validation and conflict detection. Frontend uses this for bulk import of local development changes. Validates each local change, detects conflicts with existing configurations, and loads changes into system. Returns loading results, validation status, and conflict resolution information. Critical for development workflows, configuration synchronization, and importing local changes into production system.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "load_single_local_change",
			Description: "Load specific individual local configuration change from filesystem with focused validation and immediate feedback. Frontend uses this for selective import of individual local changes. Validates single local change, checks for conflicts, and loads change into system. Returns detailed loading results, validation status, and integration information. Perfect for selective change import, testing individual modifications, and incremental development workflows.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Specific local change data including file path, component type, and modification details to load."},
				},
			},
		},

		// === METRICS SYNC ===
		{
			Name:        "metrics_sync",
			Description: "Synchronize performance metrics and monitoring data across cluster nodes for consistent system-wide monitoring and reporting. Used by monitoring systems and cluster management tools for metric aggregation and consistency. Collects and distributes performance metrics, system statistics, and monitoring data across cluster nodes. Returns synchronization status and aggregated metric data. Essential for cluster monitoring, performance analysis, and maintaining consistent metric reporting across distributed system nodes.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"data": map[string]interface{}{"type": "object", "description": "Metrics synchronization data including performance statistics, monitoring data, and system metrics."},
				},
			},
		},

		// === ERROR LOGS ===
		{
			Name:        "get_error_logs",
			Description: "Retrieve system error logs and diagnostic information from local node for troubleshooting and system monitoring. Frontend error log viewer uses this to display system errors, warnings, and diagnostic messages. Returns filtered error logs with timestamps, severity levels, component sources, and error details. Supports filtering by log level, time range, and component type. Essential for system troubleshooting, debugging component issues, and monitoring system health and stability.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        "get_cluster_error_logs",
			Description: "Retrieve aggregated error logs and diagnostic information from all cluster nodes for comprehensive system monitoring and troubleshooting. Frontend cluster error log viewer uses this to display system-wide errors and issues. Returns consolidated error logs from all cluster nodes with node identification, timestamps, severity levels, and error correlation. Supports cluster-wide filtering and error pattern analysis. Critical for cluster troubleshooting, system-wide issue detection, and comprehensive distributed system monitoring.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
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
	response, err := m.makeHTTPRequest(endpointInfo.method, endpoint, args, endpointInfo.auth)
	if err != nil {
		return common.MCPToolResult{}, err
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Type: "text",
				Text: string(response),
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
