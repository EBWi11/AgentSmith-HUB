# AgentSmith-HUB 插件使用指南

## 目录
1. [插件分类](#1-插件分类)
2. [插件语法](#2-插件语法)
3. [内置插件列表](#3-内置插件列表)
4. [在Ruleset中的使用方式](#4-在ruleset中的使用方式)
5. [自定义插件开发](#5-自定义插件开发)
6. [最佳实践](#6-最佳实践)

## 1. 插件分类

### 1.1 按运行方式分类
- **本地插件（Local Plugin）**：编译到程序中的内置插件，性能最高
- **Yaegi插件（Yaegi Plugin）**：使用Yaegi解释器运行的动态插件，**支持有状态和init函数**

### 1.2 按返回类型分类
- **检查类插件（Check Node Plugin）**：返回 `(bool, error)`，用于 `<check type="PLUGIN">` 中
- **数据处理插件（Other Plugin）**：返回 `(interface{}, bool, error)`，用于 `<append type="PLUGIN">` 和 `<plugin>` 中

## 2. 插件语法

### 2.1 基本语法
```xml
<!-- 检查类插件 -->
<check type="PLUGIN">pluginName(param1, param2, ...)</check>

<!-- 数据处理插件 -->
<append type="PLUGIN" field="field_name">pluginName(param1, param2, ...)</append>

<!-- 执行操作插件 -->
<plugin>pluginName(param1, param2, ...)</plugin>
```

### 2.2 参数类型
- **字符串**：`"value"` 或 `'value'`
- **数字**：`123` 或 `123.45`
- **布尔值**：`true` 或 `false`
- **字段引用**：`field_name` 或 `parent.child.field`
- **原始数据**：`_$ORIDATA`（唯一需要_$前缀的）

### 2.3 否定语法
检查类插件支持否定前缀：
```xml
<check type="PLUGIN">!isPrivateIP(source_ip)</check>
```

## 3. 内置插件列表

### 3.1 检查类插件（返回bool）

| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `isPrivateIP` | 检查IP是否为私有地址 | ip (string) | `isPrivateIP(source_ip)` |
| `cidrMatch` | 检查IP是否在CIDR范围内 | ip (string), cidr (string) | `cidrMatch(client_ip, "192.168.1.0/24")` |
| `geoMatch` | 检查IP所属国家 | ip (string), countryISO (string) | `geoMatch(source_ip, "US")` |
| `suppressOnce` | 告警抑制 | key (any), windowSec (int), ruleid (string, optional) | `suppressOnce(alert_key, 300, "rule_001")` |

### 3.2 数据处理插件（返回各种类型）

#### 时间处理插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `now` | 获取当前时间戳 | 可选：format (unix/ms/rfc3339) | `now()` |
| `dayOfWeek` | 获取星期几 (0-6, 0=周日) | 可选：timestamp (int64) | `dayOfWeek()` |
| `hourOfDay` | 获取小时 (0-23) | 可选：timestamp (int64) | `hourOfDay()` |
| `tsToDate` | 时间戳转RFC3339格式 | timestamp (int64) | `tsToDate(timestamp)` |

#### 编码和哈希插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `base64Encode` | Base64编码 | input (string) | `base64Encode(data)` |
| `base64Decode` | Base64解码 | input (string) | `base64Decode(encoded_data)` |
| `hashMD5` | MD5哈希 | input (string) | `hashMD5(data)` |
| `hashSHA1` | SHA1哈希 | input (string) | `hashSHA1(data)` |
| `hashSHA256` | SHA256哈希 | input (string) | `hashSHA256(data)` |

#### URL解析插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `extractDomain` | 从URL提取域名 | urlOrHost (string) | `extractDomain(url)` |
| `extractTLD` | 提取顶级域名 | domain (string) | `extractTLD(domain)` |
| `extractSubdomain` | 提取子域名 | host (string) | `extractSubdomain(host)` |

#### 字符串处理插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `replace` | 字符串替换 | input (string), old (string), new (string) | `replace(text, "old", "new")` |
| `regexExtract` | 正则提取 | input (string), pattern (string) | `regexExtract(text, "\\d+")` |
| `regexReplace` | 正则替换 | input (string), pattern (string), replacement (string) | `regexReplace(text, "\\d+", "NUMBER")` |

#### 数据解析插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `parseJSON` | 解析JSON字符串 | jsonString (string) | `parseJSON(json_data)` |
| `parseUA` | 解析User-Agent | userAgent (string) | `parseUA(user_agent)` |

#### 威胁情报插件
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| `virusTotal` | VirusTotal查询 | hash (string), apiKey (string, optional) | `virusTotal(file_hash)` |
| `shodan` | Shodan查询 | ip (string), apiKey (string, optional) | `shodan(ip_address)` |
| `threatBook` | 微步在线查询 | queryValue (string), queryType (string), apiKey (string, optional) | `threatBook(ip, "ip")` |

## 4. 在Ruleset中的使用方式

### 4.1 基本使用模式

```xml
<root author="example" type="DETECTION" name="plugin_example">
    <rule id="plugin_usage" name="Plugin Usage Examples">
        <!-- 1. 检查类插件 -->
        <check type="PLUGIN">isPrivateIP(source_ip)</check>
        
        <!-- 2. 数据处理插件 -->
        <append type="PLUGIN" field="timestamp">now()</append>
        <append type="PLUGIN" field="hash">hashSHA256(file_content)</append>
        
        <!-- 3. 执行操作插件 -->
        <plugin>sendAlert(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 4.2 复杂逻辑组合

```xml
<rule id="complex_plugin_usage" name="Complex Plugin Usage">
    <!-- 使用checklist组合多个条件 -->
    <checklist condition="(private_ip or suspicious_country) and not whitelisted">
        <check id="private_ip" type="PLUGIN">isPrivateIP(source_ip)</check>
        <check id="suspicious_country" type="PLUGIN">geoMatch(source_ip, "CN")</check>
        <check id="whitelisted" type="PLUGIN">cidrMatch(source_ip, "10.0.0.0/8")</check>
    </checklist>
    
    <!-- 数据富化 -->
    <append type="PLUGIN" field="threat_intel">virusTotal(file_hash)</append>
    <append type="PLUGIN" field="geo_info">shodan(source_ip)</append>
    
    <!-- 时间相关处理 -->
    <append type="PLUGIN" field="hour">hourOfDay()</append>
    <check type="PLUGIN">hourOfDay() > 22</check>
</rule>
```

### 4.3 告警抑制示例

```xml
<rule id="suppression_example" name="Alert Suppression">
    <check type="EQU" field="event_type">login_failed</check>
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_brute_force")</check>
    <append field="alert_type">brute_force</append>
</rule>
```

### 4.4 数据转换示例

```xml
<rule id="data_transformation" name="Data Transformation">
    <check type="EQU" field="content_type">json</check>
    
    <!-- 解析JSON并提取字段 -->
    <append type="PLUGIN" field="parsed_data">parseJSON(raw_content)</append>
    <append field="user_id">parsed_data.user.id</append>
    
    <!-- 编码处理 -->
    <append type="PLUGIN" field="encoded">base64Encode(sensitive_data)</append>
    
    <!-- 哈希计算 -->
    <append type="PLUGIN" field="content_hash">hashSHA256(raw_content)</append>
</rule>
```

## 5. 自定义插件开发

### 5.1 插件函数签名

#### 检查类插件
```go
package plugin

import (
    "errors"
    "fmt"
)

// Eval 函数必须返回 (bool, error)
func Eval(args ...interface{}) (bool, error) {
    if len(args) == 0 {
        return false, errors.New("plugin requires at least one argument")
    }
    
    // 参数处理
    data := args[0]
    
    // 插件逻辑
    if someCondition {
        return true, nil
    }
    
    return false, nil
}
```

#### 数据处理插件
```go
package plugin

import (
    "errors"
    "fmt"
)

// Eval 函数必须返回 (interface{}, bool, error)
func Eval(args ...interface{}) (interface{}, bool, error) {
    if len(args) == 0 {
        return nil, false, errors.New("plugin requires at least one argument")
    }
    
    // 参数处理
    input := args[0]
    
    // 数据处理逻辑
    result := processData(input)
    
    return result, true, nil
}
```

### 5.2 Yaegi插件的有状态特性

#### 状态保持机制
```go
// Yaegi插件支持全局变量和init函数
var (
    cache = make(map[string]interface{})
    cacheMutex sync.RWMutex
    lastUpdate time.Time
)

// init函数在插件加载时执行
func init() {
    // 初始化缓存
    refreshCache()
}

// 有状态的Eval函数
func Eval(key string) (interface{}, bool, error) {
    cacheMutex.RLock()
    if value, exists := cache[key]; exists {
        cacheMutex.RUnlock()
        return value, true, nil
    }
    cacheMutex.RUnlock()
    
    // 计算并缓存结果
    result := computeResult(key)
    cacheMutex.Lock()
    cache[key] = result
    cacheMutex.Unlock()
    
    return result, true, nil
}
```

### 5.3 插件限制
- 只能使用Go标准库
- 不能使用第三方包
- 必须定义名为`Eval`的函数
- 函数签名必须严格匹配

### 5.4 常用标准库
- 基础：`fmt`, `strings`, `strconv`, `errors`
- 编码：`encoding/json`, `encoding/base64`, `encoding/hex`
- 加密：`crypto/md5`, `crypto/sha256`, `crypto/rand`
- 时间：`time`
- 正则：`regexp`
- 网络：`net`, `net/url`
- 并发：`sync`

## 6. 最佳实践

### 6.1 性能优化
1. **执行顺序**：快速检查在前，慢速操作在后
2. **缓存使用**：利用内置插件的缓存机制
3. **避免重复计算**：合理使用字段引用

### 6.2 错误处理
1. **参数验证**：插件内部必须验证参数
2. **优雅降级**：错误时返回合理的默认值
3. **日志记录**：重要操作记录日志

### 6.3 有状态插件设计
```go
// 线程安全的状态管理
var (
    dataCache = make(map[string]interface{})
    cacheMutex sync.RWMutex
    lastRefresh time.Time
    refreshInterval = 10 * time.Minute
)

func init() {
    // 初始化缓存
    refreshCache()
    
    // 启动后台刷新任务
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            refreshCache()
        }
    }()
}

func Eval(key string) (interface{}, bool, error) {
    // 检查是否需要刷新
    if time.Since(lastRefresh) > refreshInterval {
        refreshCache()
    }
    
    cacheMutex.RLock()
    if value, exists := dataCache[key]; exists {
        cacheMutex.RUnlock()
        return value, true, nil
    }
    cacheMutex.RUnlock()
    
    return nil, false, nil
}

func refreshCache() {
    // 刷新缓存逻辑
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    
    // 执行刷新操作
    lastRefresh = time.Now()
}
```

### 6.4 调试技巧
1. **使用append跟踪**：添加调试字段
2. **分步测试**：逐个测试插件功能
3. **验证字段**：确保字段引用正确

### 6.5 常见模式

#### 缓存模式
```go
var cache = make(map[string]interface{})
var mutex sync.RWMutex

func Eval(key string) (interface{}, bool, error) {
    mutex.RLock()
    if value, exists := cache[key]; exists {
        mutex.RUnlock()
        return value, true, nil
    }
    mutex.RUnlock()
    
    // 计算并缓存
    result := expensiveCalculation(key)
    mutex.Lock()
    cache[key] = result
    mutex.Unlock()
    
    return result, true, nil
}
```

#### 计数器模式
```go
var counters = make(map[string]int)
var counterMutex sync.RWMutex

func Eval(key string, threshold int) (bool, error) {
    counterMutex.Lock()
    defer counterMutex.Unlock()
    
    counters[key]++
    return counters[key] > threshold, nil
}
```

#### 时间窗口模式
```go
var (
    lastSeen = make(map[string]time.Time)
    timeMutex sync.RWMutex
    window = 5 * time.Minute
)

func Eval(key string) (bool, error) {
    now := time.Now()
    
    timeMutex.Lock()
    defer timeMutex.Unlock()
    
    if last, exists := lastSeen[key]; exists {
        if now.Sub(last) < window {
            return false, nil // 在时间窗口内，不触发
        }
    }
    
    lastSeen[key] = now
    return true, nil // 触发
}
```

## 总结

AgentSmith-HUB的插件系统提供了强大的扩展能力：

1. **类型丰富**：支持检查类和数据处理类插件
2. **状态管理**：Yaegi插件支持有状态和init函数
3. **并发安全**：支持线程安全的状态管理
4. **性能优化**：内置缓存和后台任务支持
5. **易于开发**：清晰的函数签名和错误处理机制

通过合理使用插件，可以构建复杂的数据处理流程和业务逻辑，实现灵活的安全检测和响应机制。 