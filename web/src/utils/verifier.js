/**
 * 组件验证工具
 * 用于验证各种组件的配置是否有效
 */

import yaml from 'js-yaml'
import { parse as parseXml } from 'fast-xml-parser'

/**
 * 验证YAML格式是否有效
 * @param {string} content YAML内容
 * @returns {object} 验证结果
 */
function validateYaml(content) {
  try {
    yaml.load(content)
    return { valid: true }
  } catch (e) {
    return { 
      valid: false, 
      error: `YAML格式错误: ${e.message}` 
    }
  }
}

/**
 * 验证XML格式是否有效
 * @param {string} content XML内容
 * @returns {object} 验证结果
 */
function validateXml(content) {
  try {
    parseXml(content, {
      ignoreAttributes: false,
      attributeNamePrefix: '@_'
    })
    return { valid: true }
  } catch (e) {
    return { 
      valid: false, 
      error: `XML格式错误: ${e.message}` 
    }
  }
}

/**
 * 验证Go代码格式是否有效
 * @param {string} content Go代码内容
 * @returns {object} 验证结果
 */
function validateGo(content) {
  // 简单检查是否包含必要的函数
  const hasInitialize = content.includes('func Initialize()')
  const hasProcess = content.includes('func Process(')
  
  if (!hasInitialize && !hasProcess) {
    return {
      valid: false,
      error: '插件代码缺少必要的函数: Initialize() 和 Process()'
    }
  } else if (!hasInitialize) {
    return {
      valid: false,
      error: '插件代码缺少必要的函数: Initialize()'
    }
  } else if (!hasProcess) {
    return {
      valid: false,
      error: '插件代码缺少必要的函数: Process()'
    }
  }
  
  return { valid: true }
}

/**
 * 验证输入组件配置
 * @param {string} content 输入组件配置内容
 * @returns {object} 验证结果
 */
export function verifyInput(content) {
  // 首先验证YAML格式
  const yamlResult = validateYaml(content)
  if (!yamlResult.valid) {
    return yamlResult
  }
  
  // 解析YAML
  const config = yaml.load(content)
  
  // 检查必要字段
  if (!config.name) {
    return { valid: false, error: '缺少必要字段: name' }
  }
  
  if (!config.type) {
    return { valid: false, error: '缺少必要字段: type' }
  }
  
  // 根据不同的输入类型进行验证
  switch (config.type) {
    case 'file':
      if (!config.file || !config.file.path) {
        return { valid: false, error: '文件输入缺少必要字段: file.path' }
      }
      break
    case 'kafka':
      if (!config.kafka || !config.kafka.brokers || !config.kafka.topic) {
        return { valid: false, error: 'Kafka输入缺少必要字段: kafka.brokers 或 kafka.topic' }
      }
      break
    // 可以添加更多输入类型的验证
    default:
      return { valid: false, error: `不支持的输入类型: ${config.type}` }
  }
  
  return { valid: true }
}

/**
 * 验证输出组件配置
 * @param {string} content 输出组件配置内容
 * @returns {object} 验证结果
 */
export function verifyOutput(content) {
  // 首先验证YAML格式
  const yamlResult = validateYaml(content)
  if (!yamlResult.valid) {
    return yamlResult
  }
  
  // 解析YAML
  const config = yaml.load(content)
  
  // 检查必要字段
  if (!config.type) {
    return { valid: false, error: '缺少必要字段: type' }
  }
  
  // 根据不同的输出类型进行验证
  switch (config.type) {
    case 'file':
      if (!config.file || !config.file.path) {
        return { valid: false, error: '文件输出缺少必要字段: file.path' }
      }
      break
    case 'kafka':
      if (!config.kafka || !config.kafka.brokers || !config.kafka.topic) {
        return { valid: false, error: 'Kafka输出缺少必要字段: kafka.brokers 或 kafka.topic' }
      }
      break
    // 可以添加更多输出类型的验证
    default:
      return { valid: false, error: `不支持的输出类型: ${config.type}` }
  }
  
  return { valid: true }
}

/**
 * 验证规则集组件配置
 * @param {string} content 规则集组件配置内容
 * @returns {object} 验证结果
 */
export function verifyRuleset(content) {
  // 验证XML格式
  const xmlResult = validateXml(content)
  if (!xmlResult.valid) {
    return xmlResult
  }
  
  // 检查是否包含根元素
  if (!content.includes('<root')) {
    return { valid: false, error: '规则集缺少根元素 <root>' }
  }
  
  // 检查是否包含规则元素
  if (!content.includes('<rule')) {
    return { valid: false, error: '规则集缺少规则元素 <rule>' }
  }
  
  return { valid: true }
}

/**
 * 验证项目组件配置
 * @param {string} content 项目组件配置内容
 * @returns {object} 验证结果
 */
export function verifyProject(content) {
  // 首先验证YAML格式
  const yamlResult = validateYaml(content)
  if (!yamlResult.valid) {
    return yamlResult
  }
  
  // 解析YAML
  const config = yaml.load(content)
  
  // 检查必要字段
  if (!config.name) {
    return { valid: false, error: '缺少必要字段: name' }
  }
  
  if (!config.flow || !Array.isArray(config.flow) || config.flow.length === 0) {
    return { valid: false, error: '缺少必要字段: flow 或 flow 不是非空数组' }
  }
  
  // 验证每个流程项
  for (let i = 0; i < config.flow.length; i++) {
    const item = config.flow[i]
    if (!item.from) {
      return { valid: false, error: `流程项 #${i+1} 缺少必要字段: from` }
    }
    if (!item.to) {
      return { valid: false, error: `流程项 #${i+1} 缺少必要字段: to` }
    }
  }
  
  return { valid: true }
}

/**
 * 验证插件组件配置
 * @param {string} content 插件组件配置内容
 * @returns {object} 验证结果
 */
export function verifyPlugin(content) {
  return validateGo(content)
}

/**
 * 根据组件类型验证组件配置
 * @param {string} type 组件类型
 * @param {string} content 组件配置内容
 * @returns {object} 验证结果
 */
export function verifyComponent(type, content) {
  switch (type) {
    case 'inputs':
      return verifyInput(content)
    case 'outputs':
      return verifyOutput(content)
    case 'rulesets':
      return verifyRuleset(content)
    case 'projects':
      return verifyProject(content)
    case 'plugins':
      return verifyPlugin(content)
    default:
      return { valid: false, error: `不支持的组件类型: ${type}` }
  }
}

export default {
  verifyInput,
  verifyOutput,
  verifyRuleset,
  verifyProject,
  verifyPlugin,
  verifyComponent
} 