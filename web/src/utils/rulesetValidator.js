/**
 * Ruleset XML Validator
 * 
 * This utility provides functions to validate ruleset XML syntax
 * based on the AgentSmith-HUB rules engine requirements.
 */

// Regular expression for condition syntax validation
const conditionRegex = /^([a-z]+|\(|\)|\s)+$/;

/**
 * Validate a ruleset XML string
 * @param {string} xmlString - The ruleset XML content
 * @returns {Object} - Validation result with isValid flag and any errors
 */
export function validateRulesetXml(xmlString) {
  const result = {
    isValid: true,
    errors: [],
    warnings: []
  };

  try {
    // Parse XML
    const parser = new DOMParser();
    const xmlDoc = parser.parseFromString(xmlString, "text/xml");
    
    // Check for parsing errors
    const parserError = xmlDoc.querySelector("parsererror");
    if (parserError) {
      result.isValid = false;
      result.errors.push({
        message: "XML parsing error",
        detail: parserError.textContent,
        line: extractLineNumber(parserError.textContent)
      });
      return result;
    }

    // Validate root element
    const root = xmlDoc.querySelector("root");
    if (!root) {
      result.isValid = false;
      result.errors.push({
        message: "Missing root element",
        line: 1
      });
      return result;
    }

    // Validate root type attribute
    const rootType = root.getAttribute("type");
    if (!rootType) {
      result.warnings.push({
        message: "Root element missing type attribute, defaulting to 'DETECTION'",
        line: getLineNumber(xmlString, "<root")
      });
    } else if (rootType !== "DETECTION" && rootType !== "WHITELIST") {
      result.isValid = false;
      result.errors.push({
        message: "Root type must be 'DETECTION' or 'WHITELIST'",
        line: getLineNumber(xmlString, "<root")
      });
    }

    // Validate rules
    const rules = xmlDoc.querySelectorAll("rule");
    if (rules.length === 0) {
      result.warnings.push({
        message: "No rules defined",
        line: getLineNumber(xmlString, "<root")
      });
    }

    // Check each rule
    rules.forEach(rule => {
      validateRule(rule, result, xmlString);
    });

    // Check for duplicate rule IDs
    const ruleIds = new Map();
    rules.forEach(rule => {
      const id = rule.getAttribute("id");
      if (id) {
        if (ruleIds.has(id)) {
          result.isValid = false;
          result.errors.push({
            message: `Duplicate rule ID: ${id}`,
            line: getLineNumber(xmlString, `id="${id}"`)
          });
        } else {
          ruleIds.set(id, true);
        }
      }
    });

  } catch (error) {
    result.isValid = false;
    result.errors.push({
      message: "Validation error",
      detail: error.message,
      line: 1
    });
  }

  return result;
}

/**
 * Validate a single rule element
 * @param {Element} rule - The rule DOM element
 * @param {Object} result - The validation result object
 * @param {string} xmlString - Original XML string for line number extraction
 */
function validateRule(rule, result, xmlString) {
  const ruleId = rule.getAttribute("id");
  const ruleName = rule.getAttribute("name");
  const ruleAuthor = rule.getAttribute("author");
  const ruleLine = getLineNumber(xmlString, `<rule`);

  // Check required attributes
  if (!ruleId || ruleId.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Rule id cannot be empty",
      line: ruleLine
    });
  }

  if (!ruleName || ruleName.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Rule name cannot be empty",
      line: ruleLine
    });
  }

  if (!ruleAuthor || ruleAuthor.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Rule author cannot be empty",
      line: ruleLine
    });
  }

  // Validate filter
  const filter = rule.querySelector("filter");
  if (filter) {
    const field = filter.getAttribute("field");
    if (!field || field.trim() === "") {
      result.warnings.push({
        message: "Filter field is empty",
        line: getLineNumber(xmlString, "<filter")
      });
    }
  }

  // Validate checklist
  const checklist = rule.querySelector("checklist");
  if (checklist) {
    validateChecklist(checklist, result, xmlString, ruleId);
  } else {
    result.warnings.push({
      message: `Rule ${ruleId || ""} has no checklist`,
      line: ruleLine
    });
  }

  // Validate threshold
  const threshold = rule.querySelector("threshold");
  if (threshold) {
    validateThreshold(threshold, result, xmlString, ruleId);
  }

  // Validate appends
  const appends = rule.querySelectorAll("append");
  appends.forEach(append => {
    validateAppend(append, result, xmlString);
  });

  // Validate plugins
  const plugins = rule.querySelectorAll("plugin");
  plugins.forEach(plugin => {
    validatePlugin(plugin, result, xmlString);
  });
}

/**
 * Validate a checklist element
 * @param {Element} checklist - The checklist DOM element
 * @param {Object} result - The validation result object
 * @param {string} xmlString - Original XML string for line number extraction
 * @param {string} ruleId - The parent rule ID
 */
function validateChecklist(checklist, result, xmlString, ruleId) {
  const condition = checklist.getAttribute("condition");
  const checklistLine = getLineNumber(xmlString, "<checklist");
  
  // Check condition syntax if present
  if (condition && condition.trim() !== "") {
    if (!conditionRegex.test(condition.trim())) {
      result.isValid = false;
      result.errors.push({
        message: "Checklist condition is not a valid expression",
        detail: `Rule ID: ${ruleId}`,
        line: checklistLine
      });
    }
  }

  // Validate nodes
  const nodes = checklist.querySelectorAll("node");
  if (nodes.length === 0) {
    result.warnings.push({
      message: "Checklist has no check nodes",
      line: checklistLine
    });
    return;
  }

  // Track node IDs to check for duplicates
  const nodeIds = new Map();
  
  nodes.forEach(node => {
    const id = node.getAttribute("id");
    const type = node.getAttribute("type");
    const field = node.getAttribute("field");
    const nodeLine = getLineNumber(xmlString, "<node");
    
    // Check ID if condition is present
    if (condition && condition.trim() !== "") {
      if (!id || id.trim() === "") {
        result.isValid = false;
        result.errors.push({
          message: "Check node id cannot be empty when condition is used",
          detail: `Rule ID: ${ruleId}`,
          line: nodeLine
        });
      } else if (nodeIds.has(id)) {
        result.isValid = false;
        result.errors.push({
          message: `Duplicate node ID: ${id}`,
          detail: `Rule ID: ${ruleId}`,
          line: nodeLine
        });
      } else {
        nodeIds.set(id, true);
      }
    }
    
    // Check node type
    if (!type || type.trim() === "") {
      result.isValid = false;
      result.errors.push({
        message: "Check node type cannot be empty",
        line: nodeLine
      });
    } else {
      // Validate node type
      const validTypes = [
        "INCL", "NI", "END", "START", "NEND", "NSTART",
        "NCS_INCL", "NCS_NI", "NCS_END", "NCS_START", "NCS_NEND", "NCS_NSTART",
        "REGEX", "ISNULL", "NOTNULL", "EQU", "NEQ", "NCS_EQU", "NCS_NEQ",
        "MT", "LT", "PLUGIN"
      ];
      
      if (!validTypes.includes(type)) {
        result.isValid = false;
        result.errors.push({
          message: `Invalid node type: ${type}`,
          line: nodeLine
        });
      }
    }
    
    // Check field attribute for non-PLUGIN types
    if (type !== "PLUGIN" && (!field || field.trim() === "")) {
      result.isValid = false;
      result.errors.push({
        message: "Check node field cannot be empty",
        line: nodeLine
      });
    }
    
    // Check logic and delimiter
    const logic = node.getAttribute("logic");
    const delimiter = node.getAttribute("delimiter");
    
    if ((logic && !delimiter) || (!logic && delimiter)) {
      result.isValid = false;
      result.errors.push({
        message: "Both logic and delimiter must be specified together",
        line: nodeLine
      });
    }
    
    if (logic && logic !== "AND" && logic !== "OR") {
      result.isValid = false;
      result.errors.push({
        message: "Logic must be 'AND' or 'OR'",
        line: nodeLine
      });
    }
    
    // Check PLUGIN syntax
    if (type === "PLUGIN") {
      const value = node.textContent.trim();
      if (!value) {
        result.isValid = false;
        result.errors.push({
          message: "Plugin node value cannot be empty",
          line: nodeLine
        });
      } else if (!validatePluginSyntax(value)) {
        result.isValid = false;
        result.errors.push({
          message: "Invalid plugin function call syntax",
          detail: value,
          line: nodeLine
        });
      }
    }
    
    // Check REGEX syntax
    if (type === "REGEX") {
      try {
        const pattern = node.textContent.trim();
        new RegExp(pattern);
      } catch (e) {
        result.isValid = false;
        result.errors.push({
          message: "Invalid regex pattern",
          detail: e.message,
          line: nodeLine
        });
      }
    }
  });
}

/**
 * Validate a threshold element
 * @param {Element} threshold - The threshold DOM element
 * @param {Object} result - The validation result object
 * @param {string} xmlString - Original XML string for line number extraction
 * @param {string} ruleId - The parent rule ID
 */
function validateThreshold(threshold, result, xmlString, ruleId) {
  const groupBy = threshold.getAttribute("group_by");
  const range = threshold.getAttribute("range");
  const countType = threshold.getAttribute("count_type");
  const countField = threshold.getAttribute("count_field");
  const thresholdValue = threshold.textContent.trim();
  const thresholdLine = getLineNumber(xmlString, "<threshold");
  
  // Check required attributes
  if (!groupBy || groupBy.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Threshold group_by cannot be empty",
      detail: `Rule ID: ${ruleId}`,
      line: thresholdLine
    });
  }
  
  if (!range || range.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Threshold range cannot be empty",
      detail: `Rule ID: ${ruleId}`,
      line: thresholdLine
    });
  } else {
    // Validate time range format
    if (!validateTimeRange(range)) {
      result.isValid = false;
      result.errors.push({
        message: "Invalid time range format",
        detail: `Expected format like '30s', '5m', '1h', got '${range}'`,
        line: thresholdLine
      });
    }
  }
  
  // Check threshold value
  if (!thresholdValue || isNaN(parseInt(thresholdValue)) || parseInt(thresholdValue) <= 1) {
    result.isValid = false;
    result.errors.push({
      message: "Threshold value must be greater than 1",
      detail: `Rule ID: ${ruleId}`,
      line: thresholdLine
    });
  }
  
  // Validate count_type if present
  if (countType && countType !== "SUM" && countType !== "CLASSIFY") {
    result.isValid = false;
    result.errors.push({
      message: "Threshold count_type must be 'SUM' or 'CLASSIFY'",
      detail: `Rule ID: ${ruleId}`,
      line: thresholdLine
    });
  }
  
  // Check count_field if count_type is specified
  if ((countType === "SUM" || countType === "CLASSIFY") && (!countField || countField.trim() === "")) {
    result.isValid = false;
    result.errors.push({
      message: "Threshold count_field cannot be empty when count_type is specified",
      detail: `Rule ID: ${ruleId}`,
      line: thresholdLine
    });
  }
}

/**
 * Validate an append element
 * @param {Element} append - The append DOM element
 * @param {Object} result - The validation result object
 * @param {string} xmlString - Original XML string for line number extraction
 */
function validateAppend(append, result, xmlString) {
  const type = append.getAttribute("type");
  const fieldName = append.getAttribute("field_name");
  const value = append.textContent.trim();
  const appendLine = getLineNumber(xmlString, "<append");
  
  if (!fieldName || fieldName.trim() === "") {
    result.isValid = false;
    result.errors.push({
      message: "Append field_name cannot be empty",
      line: appendLine
    });
  }
  
  if (type === "PLUGIN") {
    if (!value) {
      result.isValid = false;
      result.errors.push({
        message: "Append plugin value cannot be empty",
        line: appendLine
      });
    } else if (!validatePluginSyntax(value)) {
      result.isValid = false;
      result.errors.push({
        message: "Invalid plugin function call syntax",
        detail: value,
        line: appendLine
      });
    }
  }
}

/**
 * Validate a plugin element
 * @param {Element} plugin - The plugin DOM element
 * @param {Object} result - The validation result object
 * @param {string} xmlString - Original XML string for line number extraction
 */
function validatePlugin(plugin, result, xmlString) {
  const value = plugin.textContent.trim();
  const pluginLine = getLineNumber(xmlString, "<plugin");
  
  if (!value) {
    result.isValid = false;
    result.errors.push({
      message: "Plugin value cannot be empty",
      line: pluginLine
    });
  } else if (!validatePluginSyntax(value)) {
    result.isValid = false;
    result.errors.push({
      message: "Invalid plugin function call syntax",
      detail: value,
      line: pluginLine
    });
  }
}

/**
 * Validate plugin function call syntax
 * @param {string} input - The plugin function call string
 * @returns {boolean} - Whether the syntax is valid
 */
function validatePluginSyntax(input) {
  const funcCallRegex = /^([a-zA-Z_][a-zA-Z0-9_]*)\s*\((.*)\)$/;
  const matches = funcCallRegex.exec(input.trim());
  
  if (!matches || matches.length !== 3) {
    return false;
  }
  
  return true;
}

/**
 * Validate time range format
 * @param {string} range - The time range string
 * @returns {boolean} - Whether the format is valid
 */
function validateTimeRange(range) {
  const timeRangeRegex = /^(\d+)(s|m|h|d)$/;
  return timeRangeRegex.test(range.trim());
}

/**
 * Extract line number from parser error message
 * @param {string} errorMessage - The parser error message
 * @returns {number} - The extracted line number or 1 if not found
 */
function extractLineNumber(errorMessage) {
  const lineMatch = errorMessage.match(/line\s+(\d+)/i);
  return lineMatch ? parseInt(lineMatch[1]) : 1;
}

/**
 * Get line number for a string pattern in the XML
 * @param {string} xml - The XML string
 * @param {string} pattern - The pattern to search for
 * @returns {number} - The line number (1-based)
 */
function getLineNumber(xml, pattern) {
  const lines = xml.split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].includes(pattern)) {
      return i + 1;
    }
  }
  return 1;
} 