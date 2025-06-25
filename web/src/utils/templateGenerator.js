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
  return `<root type="DETECTION">
    <rule id="${id}_01" name="Example Rule">
        <filter field="data_type">59</filter>
        <checklist condition="a and (b or c)">
            <node id="a" type="INCL" field="data" logic="or" delimiter="|">test1|test2</node>
            <node id="b" type="REGEX" field="data">^example.*pattern$</node>
            <node id="c" type="PLUGIN">plugin_name(_$ORIDATA)</node>
        </checklist>

        <threshold group_by="exe,data_type" range="30s" local_cache="true" count_type="SUM" count_field="dip">5</threshold>

        <append field="data_type">10</append>
        <append type="PLUGIN" field="data_type">plugin_name(_$ORIDATA)</append>
        
        <plugin>plugin_name(_$ORIDATA)</plugin>
        <del>sport,dport</del>
    </rule>
</root>`;
}

/**
 * Generate a template for a new input component
 * @param {string} id - The ID of the input component
 * @returns {string} - YAML template for the input
 */
export function generateInputTemplate(id) {
  return `name: ${id}
type: kafka
config:
  brokers:
    - localhost:9092
  topics:
    - test-topic
  group_id: ${id}-consumer
  auto_offset_reset: earliest`;
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
  // 尝试获取实际的组件名称
  let inputExample = 'example_input';
  let rulesetExample = 'example_ruleset';
  let outputExample = 'example_output';
  
  // 如果提供了store，尝试获取实际组件
  if (store) {
    const inputs = store.getters.getComponents('inputs');
    const rulesets = store.getters.getComponents('rulesets');
    const outputs = store.getters.getComponents('outputs');
    
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
	"fmt"
)

// ${id} is an example plugin
// It takes a data map and returns a boolean result
func ${id}(data interface{}) (bool, string) {
	// Your plugin logic here
	fmt.Println("Plugin ${id} called with data:", data)
	
	// Example implementation
	return true, "Plugin executed successfully"
}

// Register the plugin when the package is imported
func init() {
	Plugins["${id}"] = ${id}
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