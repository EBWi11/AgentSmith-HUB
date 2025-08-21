/**
 * Template Generator
 * 
 * This utility provides template generation functions for various component types.
 */

/**
 * Generate a template for a new ruleset
 * @param {string} id - The ID of the ruleset
 * @returns {string} - XML template for the ruleset
 */
export function generateRulesetTemplate(id) {
  return `<root type="DETECTION" name="${id}">
    <rule id="${id}_01" name="Example Rule">
        <!-- Operations can be placed in any order -->
        
        <!-- Standalone check examples -->
        <check type="EQU" field="event_type">login</check>
        <check type="INCL" field="source" logic="OR" delimiter="|">api|web|mobile</check>
        
        <!-- Threshold example -->
        <threshold group_by="user_id,ip" range="5m">10</threshold>
        
        <!-- Checklist with conditional logic (supports check and threshold nodes) -->
        <checklist condition="a and (b or c)">
            <check id="a" type="INCL" field="data" logic="OR" delimiter="|">test1|test2</check>
            <check id="b" type="REGEX" field="data">^example.*pattern$</check>
            <check id="c" type="MT" field="score">80</check>
            <threshold id="threshold_c" group_by="user_id" range="5m">10</threshold>
        </checklist>

        <!-- Iterator example -->
        <iterator type="ALL" field="data" variable="item">
            <check type="EQU" field="item.a">value</check>
            <checklist condition="a and (b or c)">
                <check id="a" type="INCL" field="item.a" logic="OR" delimiter="|">test1|test2</check>
                <check id="b" type="REGEX" field="item.b">^example.*pattern$</check>
                <check id="c" type="MT" field="item.c">80</check>
            </checklist>
            <threshold id="threshold_c" group_by="item.a" range="5m">10</threshold>
        </iterator>
        
        <!-- Additional operations -->
        <append field="risk_level">high</append>
        <append type="PLUGIN" field="timestamp">now()</append>
        
        <plugin>alert("security_team")</plugin>
        
        <del>temp_field,debug_info</del>
    </rule>
</root>`;
}

/**
 * Generate a template for a new input component
 * @param {string} id - The ID of the input component
 * @returns {string} - YAML template for the input
 */
export function generateInputTemplate(id) {
  return `type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "${id}-consumer"
  topic: "test-topic"
  compression: "none"
  # offset_reset controls where to start consuming when no committed offset exists
  # earliest: start from the beginning of the topic (default, recommended)
  # latest: start from the end of the topic (only new messages)
  # none: fail if no committed offset exists
  offset_reset: earliest
  # Uncomment below for SASL authentication
  # sasl:
  #   enable: true
  #   mechanism: "plain"  # plain, scram-sha256, scram-sha512
  #   username: "your_username"
  #   password: "your_password"
  # Uncomment below for TLS configuration
  # tls:
  #   cert_path: "/path/to/client.crt"
  #   key_path: "/path/to/client.key"
  #   ca_file_path: "/path/to/ca.crt"
  #   skip_verify: false

# Alternative Aliyun SLS input example:
# type: aliyun_sls
# aliyun_sls:
#   endpoint: "https://your-project.your-region.log.aliyuncs.com"
#   access_key_id: "your_access_key_id"
#   access_key_secret: "your_access_key_secret"
#   project: "your_project"
#   logstore: "your_logstore"
#   consumer_group_name: "${id}-consumer"
#   consumer_name: "${id}"
#   cursor_position: "begin"  # begin, end, or specific timestamp
#   # cursor_start_time: 1640995200000  # Unix timestamp in milliseconds
#   # query: "*"  # Optional query for filtering logs`;
}

/**
 * Generate a template for a new output component
 * @param {string} id - The ID of the output component
 * @returns {string} - YAML template for the output
 */
export function generateOutputTemplate(id) {
  return `name: ${id}
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  topic: "output-topic"
  compression: "none"
  # Uncomment below for SASL authentication
  # sasl:
  #   enable: true
  #   mechanism: "plain"
  #   username: "your_username"
  #   password: "your_password"

# Alternative Elasticsearch output example:
# name: ${id}
# type: elasticsearch
# elasticsearch:
#   hosts:
#     - "https://localhost:9200"  # HTTPS supported, TLS cert verification skipped by default
#   index: "${id}-index"
#   batch_size: 1000
#   flush_dur: "5s"
#   # Uncomment below for authentication
#   # auth:
#   #   type: basic  # or api_key, bearer
#   #   username: "elastic"
#   #   password: "password"
#   #   # For API key auth:
#   #   # api_key: "your-api-key"
#   #   # For bearer token auth:
#   #   # token: "your-bearer-token"`;
}

/**
 * Generate a template for a new project component
 * @param {string} id - The ID of the project
 * @param {Object} store - Vuex store for accessing component lists
 * @returns {string} - YAML template for the project
 */
export function generateProjectTemplate(id, store) {
  // Try to get actual component names
  let inputExample = 'example_input';
  let rulesetExample = 'example_ruleset';
  let outputExample = 'example_output';
  
  // If store is provided, try to get actual components from dataCache
  if (store && store.$dataCache) {
    const inputs = store.$dataCache.getComponentData('inputs');
    const rulesets = store.$dataCache.getComponentData('rulesets');
    const outputs = store.$dataCache.getComponentData('outputs');
    
    if (inputs && inputs.length > 0) {
      inputExample = inputs[0].id;
    }
    
    if (rulesets && rulesets.length > 0) {
      rulesetExample = rulesets[0].id;
    }
    
    if (outputs && outputs.length > 0) {
      outputExample = outputs[0].id;
    }
  }
  
  return `name: ${id}
flow:
  - from: "input.${inputExample}"
    to: "ruleset.${rulesetExample}"
  - from: "ruleset.${rulesetExample}"
    to: "output.${outputExample}"`;
}

/**
 * Generate a template for a new plugin component
 * @param {string} id - The ID of the plugin
 * @returns {string} - Go template for the plugin
 */
export function generatePluginTemplate(id) {
  return `package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// ${id} is a plugin for AgentSmith-HUB
// It takes arguments and returns a boolean result for checknode usage
func Eval(args ...interface{}) (bool, error) {
	// Input validation - always check arguments first
	if len(args) == 0 {
		return false, errors.New("plugin requires at least one argument")
	}
	
	// Get the first argument (typically data or _$ORIDATA)
	data := args[0]
	
	// Example: Convert to string for processing
	dataStr := fmt.Sprintf("%v", data)
	
	// Your plugin logic here
	// Example implementation: check if data contains specific string
	if strings.Contains(dataStr, "example") {
		return true, nil
	}
	
	return false, nil
}`;
}

/**
 * Get a template for a new component based on its type
 * @param {string} type - The component type (rulesets, inputs, outputs, projects, plugins)
 * @param {string} id - The ID of the component
 * @param {Object} store - Optional Vuex store for accessing component lists
 * @returns {string} - Template for the component
 */
export function getDefaultTemplate(type, id, store) {
  switch (type) {
    case 'rulesets':
      return generateRulesetTemplate(id);
    case 'inputs':
      return generateInputTemplate(id);
    case 'outputs':
      return generateOutputTemplate(id);
    case 'projects':
      return generateProjectTemplate(id, store);
    case 'plugins':
      return generatePluginTemplate(id);
    default:
      return `# New ${type} component: ${id}\n`;
  }
}