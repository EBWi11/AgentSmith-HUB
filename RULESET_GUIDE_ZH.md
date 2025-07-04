# 🛡️ AgentSmith-HUB 规则引擎完整指南
## 🚀 快速上手

### 什么是 AgentSmith-HUB 规则引擎？

AgentSmith-HUB 规则引擎是一个基于XML配置的实时数据处理引擎，用于：
- **数据过滤**：根据条件筛选数据
- **威胁检测**：识别安全威胁和异常行为
- **数据转换**：对数据进行加工和处理
- **实时分析**：对数据流进行实时监控

```
输入数据 → 过滤器(Filter) → 检查列表(CheckList) → 阈值检测(Threshold) → 数据处理(Append/Del/Plugin) → 输出数据
```

### 1分钟写出第一个规则

```xml
<root type="DETECTION" name="my_first_ruleset" author="your_name">
    <rule id="detect_powershell" name="检测PowerShell执行">
        <!-- 1. 过滤器：只处理进程创建事件 -->
        <filter field="event_type">process_creation</filter>

        <!-- 2. 检查列表：检查进程名是否包含powershell -->
        <checklist>
            <node type="INCL" field="process_name">powershell</node>
        </checklist>

        <!-- 3. 添加字段：标记为可疑活动 -->
        <append field="alert_type">suspicious_powershell</append>
    </rule>
</root>
```

**这个规则做了什么？**
1. 监听所有进程创建事件（event_type = process_creation）
2. 检查进程名（process_name字段）是否包含（INCL）"powershell"
3. 如果匹配，添加一个`alert_type`字段标记为可疑活动(suspicious_powershell)

---

## 🧠 核心概念

### 规则集(Ruleset)类型
- **DETECTION**：检测规则集，匹配到后数据向后传递，不填写默认为 DETECTION
- **WHITELIST**：白名单规则集，匹配时丢弃数据，未匹配到的数据向后传递；白名单规则不支持 `<append>`、`<del>`、`<plugin>`等数据处理操作

### 规则(Rule)执行流程
1. **Filter过滤**：快速过滤不相关数据
2. **CheckList检查**：执行具体的检测逻辑
3. **Threshold阈值**：统计频率和数量
4. **数据处理**：添加（Append）、删除（Del）字段或执行插件（Plugin）

### 性能优化机制
- **自动排序**：系统自动按性能优化节点执行顺序
- **智能缓存**：缓存常用计算结果
- **动态线程调整**：随着规则引擎负载自动调整线程数
- **正则优化**：使用高性能正则引擎

---

## 📋 字段详解

### Root根元素
```xml
<root type="DETECTION" name="ruleset_name" author="author_name">
```

| 属性 | 必需 | 说明           | 可选值 |
|------|----|--------------|--------|
| `type` | 否  | 规则集类型，不填默认为'DETECTION' | `DETECTION`, `WHITELIST` |
| `name` | 否  | 规则集名称        | 任意字符串 |
| `author` | 否  | 作者信息         | 任意字符串 |

### Rule规则元素
```xml
<rule id="unique_rule_id" name="readable_rule_name">
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| `id` | 是 | 规则唯一标识符 | `detect_malware_01` |
| `name` | 否 | 规则可读名称 | `检测恶意软件` |

### Filter过滤器
```xml
<filter field="field_name">value</filter>
```

| 属性 | 必需 | 说明 | 示例 |
|------|----|------|------|
| `field` | 是  | 要过滤的字段名 | `event_type`, `data_type` |

**用途**：在执行复杂检查前快速过滤数据，显著提升性能；filter 本身不是必填项

### CheckList检查列表
```xml
<checklist condition="logic_expression">
    <node id="node_id" type="check_type" field="field_name">value</node>
</checklist>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| `condition` | 否 | 逻辑表达式 | `a and (b or c)` |

**逻辑表达式语法**：
- `and`：逻辑与
- `or`：逻辑或
- `()`：分组
- 节点ID：引用具体检查节点

### Node检查节点
```xml
<node id="node_id" type="check_type" field="field_name" logic="OR" delimiter="|">value</node>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| `id` | 条件 | 节点标识符（使用condition时必需） | `check_process` |
| `type` | 是 | 检查类型 | `INCL`, `EQU`, `REGEX`等 |
| `field` | 条件 | 要检查的字段名（PLUGIN类型可选） | `process_name` |
| `logic` | 否 | 多值逻辑 | `OR`, `AND` |
| `delimiter` | 条件 | 分隔符（使用logic时必需） | `|`, `,` |

### Threshold阈值检测
```xml
<threshold group_by="field1,field2" range="300s" count_type="SUM" count_field="amount" local_cache="true">10</threshold>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| `group_by` | 是 | 分组字段 | `source_ip,user_id` |
| `range` | 是 | 时间范围 | `300s`, `5m`, `1h` |
| `count_type` | 否 | 计数类型 | `SUM`, `CLASSIFY` |
| `count_field` | 条件 | 计数字段（SUM/CLASSIFY时必需） | `bytes`, `resource_id` |
| `local_cache` | 否 | 使用本地缓存 | `true`, `false` |

### Append字段追加
```xml
<append field="new_field" type="PLUGIN">value_or_plugin_call</append>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| `field` | 是 | 要添加的字段名 | `alert_level`, `geo_info` |
| `type` | 否 | 追加类型 | `PLUGIN` |

### Plugin插件执行
```xml
<plugin>plugin_function(arg1, arg2)</plugin>
```

**用途**：执行副作用操作，如发送告警、记录日志等。

### Del字段删除
```xml
<del>field1,field2,field3</del>
```

**用途**：删除不需要的字段，减少内存占用。

---

## 🎯 节点类型完整参考

### 字符串匹配类（高性能）

| 类型 | 功能 | 示例 | 说明 |
|------|------|------|------|
| `EQU` | 完全相等 | `<node type="EQU" field="status">active</node>` | 大小写敏感 |
| `NEQ` | 完全不等 | `<node type="NEQ" field="user">guest</node>` | 大小写敏感 |
| `INCL` | 包含子串 | `<node type="INCL" field="path">/admin/</node>` | 大小写敏感 |
| `NI` | 不包含子串 | `<node type="NI" field="agent">bot</node>` | 大小写敏感 |
| `START` | 开头匹配 | `<node type="START" field="cmd">powershell</node>` | 大小写敏感 |
| `END` | 结尾匹配 | `<node type="END" field="file">.exe</node>` | 大小写敏感 |
| `NSTART` | 开头不匹配 | `<node type="NSTART" field="path">C:\Windows</node>` | 大小写敏感 |
| `NEND` | 结尾不匹配 | `<node type="NEND" field="file">.tmp</node>` | 大小写敏感 |

### 大小写忽略类（高性能）

| 类型 | 功能 | 示例 |
|------|------|------|
| `NCS_EQU` | 忽略大小写相等 | `<node type="NCS_EQU" field="browser">CHROME</node>` |
| `NCS_NEQ` | 忽略大小写不等 | `<node type="NCS_NEQ" field="os">windows</node>` |
| `NCS_INCL` | 忽略大小写包含 | `<node type="NCS_INCL" field="domain">SUSPICIOUS</node>` |
| `NCS_NI` | 忽略大小写不包含 | `<node type="NCS_NI" field="referrer">GOOGLE</node>` |
| `NCS_START` | 忽略大小写开头 | `<node type="NCS_START" field="cmd">POWERSHELL</node>` |
| `NCS_END` | 忽略大小写结尾 | `<node type="NCS_END" field="script">.PS1</node>` |
| `NCS_NSTART` | 忽略大小写开头不匹配 | `<node type="NCS_NSTART" field="user">ADMIN</node>` |
| `NCS_NEND` | 忽略大小写结尾不匹配 | `<node type="NCS_NEND" field="domain">TRUSTED</node>` |

### 数值比较类（高性能）

| 类型 | 功能 | 示例 |
|------|------|------|
| `MT` | 大于 | `<node type="MT" field="score">75.5</node>` |
| `LT` | 小于 | `<node type="LT" field="cpu_usage">90</node>` |

### 空值检查类（最高性能）

| 类型 | 功能 | 示例 |
|------|------|------|
| `ISNULL` | 字段为空 | `<node type="ISNULL" field="optional_field"></node>` |
| `NOTNULL` | 字段非空 | `<node type="NOTNULL" field="required_field"></node>` |

### 正则表达式类（低性能）

| 类型 | 功能 | 示例 |
|------|------|------|
| `REGEX` | 正则匹配 | `<node type="REGEX" field="ip">^192\.168\.\d+\.\d+$</node>` |

### 插件调用类（最低性能）

| 类型 | 功能 | 示例 |
|------|------|------|
| `PLUGIN` | 插件函数 | `<node type="PLUGIN">is_malicious_domain(domain_name)</node>` |

### 多值匹配

```xml
<!-- OR逻辑：匹配任意一个值 -->
<node type="INCL" field="process" logic="OR" delimiter="|">malware.exe|virus.exe|trojan.exe</node>

        <!-- AND逻辑：必须包含所有值 -->
<node type="INCL" field="command" logic="AND" delimiter="|">-exec|-payload</node>
```

---

## 🔌 插件系统详解

### 插件基础概念

插件是扩展规则引擎功能的重要机制，允许执行复杂的自定义逻辑。

### 插件类型

#### 1. CheckNode插件（检查节点插件）
用于复杂的条件判断，必须返回布尔值。

```xml
<checklist>
    <node type="PLUGIN">is_suspicious_ip(source_ip)</node>
    <node type="PLUGIN">is_malicious_domain(domain_name)</node>
    <node type="PLUGIN">check_user_behavior(_$user_id, _$recent_activities)</node>
</checklist>
```

**特点**：
- 必须返回`bool`类型
- 用于条件判断
- 可与其他节点组合使用

#### 2. Append插件（字段追加插件）
用于生成新的字段值，可返回任意类型。

```xml
<append type="PLUGIN" field="geo_location">get_geolocation(source_ip)</append>
<append type="PLUGIN" field="threat_score">calculate_threat_score(_$ORIDATA)</append>
<append type="PLUGIN" field="user_profile">get_user_info(_$user_id)</append>
```

**特点**：
- 可返回任意类型（字符串、数字、对象等）
- 用于数据丰富化
- 结果作为新字段添加到数据中

#### 3. Standalone插件（独立插件）
用于执行副作用操作，返回值被忽略。

```xml
<plugin>send_alert(_$ORIDATA, "HIGH")</plugin>
<plugin>log_security_event(_$ORIDATA)</plugin>
<plugin>update_threat_intelligence(_$indicators)</plugin>
```

**特点**：
- 返回值被忽略
- 用于副作用操作
- 如发送告警、记录日志、更新数据库等

### 插件参数类型

#### 1. 字面量参数
```xml
<node type="PLUGIN">check_threshold("high", 100, true)</node>
```

支持的字面量类型：
- 字符串：`"hello"` 或 `'hello'`
- 数字：`123`, `45.67`
- 布尔值：`true`, `false`

#### 2. 字段引用参数
```xml
<node type="PLUGIN">validate_user(user_id, session_token)</node>
```

直接引用数据中的字段值。

#### 3. FromRawSymbol参数
```xml
<node type="PLUGIN">analyze_behavior(_$user.profile.id, _$session.activities)</node>
```

使用`_$`前缀引用数据中的字段，支持嵌套访问。

#### 4. 原始数据参数
```xml
<node type="PLUGIN">complex_analysis(_$ORIDATA)</node>
```

`_$ORIDATA`代表完整的原始数据。

### 插件开发指南

#### Go插件示例
```go
package main

import (
	"fmt"
	"strings"
)

// CheckNode插件：检查IP是否可疑
func IsSuspiciousIP(ip string) bool {
	// 检查是否为内网IP
	if strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "172.") {
		return false
	}

	// 检查是否在黑名单中
	blacklist := []string{"1.2.3.4", "5.6.7.8"}
	for _, blocked := range blacklist {
		if ip == blocked {
			return true
		}
	}

	return false
}

// Append插件：获取地理位置信息
func GetGeolocation(ip string) map[string]interface{} {
	// 模拟地理位置查询
	return map[string]interface{}{
		"country": "US",
		"city": "New York",
		"latitude": 40.7128,
		"longitude": -74.0060,
	}
}

// Standalone插件：发送告警
func SendAlert(data map[string]interface{}, level string) {
	fmt.Printf("ALERT [%s]: %v\n", level, data)
	// 实际实现中会调用告警系统API
}
```

#### 插件注册
```go
// 在插件系统中注册函数
func init() {
RegisterPlugin("is_suspicious_ip", IsSuspiciousIP)
RegisterPlugin("get_geolocation", GetGeolocation)
RegisterPlugin("send_alert", SendAlert)
}
```

### 插件最佳实践

#### 1. 性能优化
```xml
<!-- 好：先用高性能节点过滤，再用插件 -->
<checklist condition="basic_check and plugin_check">
    <node id="basic_check" type="INCL" field="process_name">suspicious</node>
    <node id="plugin_check" type="PLUGIN">deep_analysis(_$ORIDATA)</node>
</checklist>

        <!-- 不好：直接使用插件 -->
<checklist>
<node type="PLUGIN">complex_analysis(_$ORIDATA)</node>
</checklist>
```

#### 2. 错误处理
```xml
<!-- 插件应该优雅处理错误 -->
<checklist condition="safe_check and plugin_check">
    <node id="safe_check" type="NOTNULL" field="required_field"></node>
    <node id="plugin_check" type="PLUGIN">safe_analysis(_$required_field)</node>
</checklist>
```

#### 3. 数据验证
```go
func SafeAnalysis(data interface{}) bool {
// 验证输入数据
if data == nil {
return false
}

// 类型断言
str, ok := data.(string)
if !ok {
return false
}

// 执行分析
return analyzeString(str)
}
```

---

## 🚀 高级特性

### FromRawSymbol动态字段

#### 基础用法
```xml
<!-- 静态值 -->
<node type="EQU" field="status">active</node>

        <!-- 动态值：从数据中获取 -->
<node type="EQU" field="status">_$expected_status</node>
```

#### 嵌套字段访问
```xml
<!-- 访问嵌套字段 -->
<node type="EQU" field="user_level">_$user.profile.security_level</node>
<filter field="event.source.system">_$config.target_system</filter>
```

#### 在不同元素中使用
```xml
<rule id="dynamic_rule" name="动态规则示例">
    <!-- Filter中使用 -->
    <filter field="event_type">_$monitoring.target_event</filter>

    <!-- CheckList中使用 -->
    <checklist>
        <node type="MT" field="risk_score">_$thresholds.min_risk</node>
        <node type="INCL" field="user_group">_$policies.allowed_groups</node>
    </checklist>

    <!-- Threshold中使用 -->
    <threshold group_by="_$grouping.primary_field" range="300s">_$limits.max_count</threshold>

    <!-- Append中使用 -->
    <append field="processing_time">_$event.timestamp</append>
</rule>
```

### 阈值检测详解

#### 默认计数模式
```xml
<threshold group_by="source_ip,user_id" range="300s" local_cache="true">5</threshold>
```

**用途**：统计事件发生次数
**示例**：5分钟内同一IP和用户的失败登录超过5次

#### SUM聚合模式
```xml
<threshold group_by="account_id" range="86400s" count_type="SUM" count_field="amount">50000</threshold>
```

**用途**：统计数值字段的总和
**示例**：24小时内同一账户的交易总额超过50000

#### CLASSIFY唯一计数模式
```xml
<threshold group_by="user_id" range="3600s" count_type="CLASSIFY" count_field="resource_id">25</threshold>
```

**用途**：统计唯一值的数量
**示例**：1小时内同一用户访问超过25个不同资源

### 复杂逻辑表达式

#### 基础逻辑
```xml
<!-- AND逻辑（默认） -->
<checklist>
    <node type="INCL" field="process">malware</node>
    <node type="INCL" field="path">temp</node>
</checklist>

        <!-- OR逻辑 -->
<checklist condition="a or b">
<node id="a" type="INCL" field="process">malware</node>
<node id="b" type="INCL" field="path">suspicious</node>
</checklist>
```

#### 复杂组合
```xml
<checklist condition="(threat_detected or anomaly_detected) and not whitelisted">
    <node id="threat_detected" type="PLUGIN">detect_threat(_$ORIDATA)</node>
    <node id="anomaly_detected" type="MT" field="anomaly_score">0.8</node>
    <node id="whitelisted" type="PLUGIN">is_whitelisted(_$source_ip)</node>
</checklist>
```

### XML特殊字符处理

#### 使用CDATA
```xml
<!-- 错误：包含XML特殊字符 -->
<node type="REGEX" field="html"><script>alert('xss')</script></node>

        <!-- 正确：使用CDATA -->
<node type="REGEX" field="html"><![CDATA[<script>alert('xss')</script>]]></node>

        <!-- 复杂正则表达式 -->
<node type="REGEX" field="sql_query"><![CDATA[(?i)(union\s+select|insert\s+into|drop\s+table)]]></node>
```

**何时使用CDATA**：
- 包含 `<` `>` `&` `"` `'` 字符时
- 复杂的正则表达式
- HTML/XML内容匹配

---

## 💡 实战案例

### 案例1：恶意PowerShell检测

```xml
<root type="DETECTION" name="powershell_detection" author="security_team">
    <rule id="malicious_powershell" name="恶意PowerShell检测">
        <!-- 过滤：只处理进程创建事件 -->
        <filter field="event_type">process_creation</filter>

        <!-- 检查：PowerShell + 可疑参数 -->
        <checklist condition="powershell_proc and (encoded_cmd or bypass_policy or download_cradle)">
            <node id="powershell_proc" type="INCL" field="process_name">powershell</node>
            <node id="encoded_cmd" type="INCL" field="command_line">-EncodedCommand</node>
            <node id="bypass_policy" type="INCL" field="command_line">-ExecutionPolicy Bypass</node>
            <node id="download_cradle" type="PLUGIN">detect_download_cradle(_$command_line)</node>
        </checklist>

        <!-- 阈值：10分钟内同一主机超过3次 -->
        <threshold group_by="hostname" range="600s" local_cache="true">3</threshold>

        <!-- 数据丰富化 -->
        <append field="alert_type">malicious_powershell</append>
        <append field="severity">high</append>
        <append type="PLUGIN" field="decoded_command">decode_powershell(_$command_line)</append>

        <!-- 执行响应动作 -->
        <plugin>send_alert(_$ORIDATA, "HIGH")</plugin>
        <plugin>isolate_host_if_confirmed(_$hostname, _$confidence_score)</plugin>

        <!-- 清理敏感信息 -->
        <del>raw_log,internal_metadata</del>
    </rule>
</root>
```

### 案例2：Web攻击检测

```xml
<root type="DETECTION" name="web_security" author="security_team">
    <rule id="sql_injection" name="SQL注入检测">
        <filter field="event_type">web_request</filter>

        <checklist condition="sql_patterns and not false_positive">
            <node id="sql_patterns" type="REGEX" field="request_body"><![CDATA[(?i)(union\s+select|insert\s+into|delete\s+from|drop\s+table|exec\s*\(|xp_cmdshell)]]></node>
            <node id="false_positive" type="PLUGIN">is_legitimate_request(_$request_context)</node>
        </checklist>

        <threshold group_by="source_ip" range="300s">5</threshold>

        <append field="attack_type">sql_injection</append>
        <append type="PLUGIN" field="payload_analysis">analyze_sql_payload(_$request_body)</append>

        <plugin>block_ip(_$source_ip)</plugin>
        <plugin>alert_security_team(_$ORIDATA)</plugin>
    </rule>

    <rule id="xss_detection" name="XSS攻击检测">
        <filter field="event_type">web_request</filter>

        <checklist>
            <node type="REGEX" field="request_params"><![CDATA[(?i)(<script[^>]*>|javascript:|on\w+\s*=|eval\s*\(|alert\s*\()]]></node>
        </checklist>

        <threshold group_by="source_ip,target_url" range="600s">3</threshold>

        <append field="attack_type">cross_site_scripting</append>
        <append type="PLUGIN" field="xss_payload">extract_xss_payload(_$request_params)</append>

        <plugin>sanitize_and_log(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 案例3：金融欺诈检测

```xml
<root type="DETECTION" name="fraud_detection" author="fraud_team">
    <rule id="suspicious_transaction" name="可疑交易检测">
        <filter field="event_type">financial_transaction</filter>

        <checklist condition="large_amount and (velocity_anomaly or location_anomaly or time_anomaly)">
            <node id="large_amount" type="MT" field="amount">_$user.daily_limit</node>
            <node id="velocity_anomaly" type="PLUGIN">detect_velocity_anomaly(_$user_id, _$amount)</node>
            <node id="location_anomaly" type="PLUGIN">detect_location_anomaly(_$user_id, _$location)</node>
            <node id="time_anomaly" type="PLUGIN">detect_time_anomaly(_$user_id, _$timestamp)</node>
        </checklist>

        <!-- 24小时内交易总额阈值 -->
        <threshold group_by="user_id" range="86400s" count_type="SUM" count_field="amount">_$user.daily_limit</threshold>

        <append field="fraud_type">suspicious_transaction</append>
        <append type="PLUGIN" field="risk_score">calculate_risk_score(_$ORIDATA)</append>
        <append type="PLUGIN" field="recommended_action">determine_action(_$risk_score)</append>

        <plugin>freeze_account_if_high_risk(_$user_id, _$risk_score)</plugin>
        <plugin>notify_fraud_team(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 案例4：网络威胁检测

```xml
<root type="DETECTION" name="network_threat" author="security_team">
    <rule id="c2_communication" name="C2通信检测">
        <filter field="event_type">network_connection</filter>

        <checklist condition="external_connection and (suspicious_port or known_malware_domain or beacon_pattern)">
            <node id="external_connection" type="PLUGIN">is_external_connection(_$dest_ip)</node>
            <node id="suspicious_port" type="INCL" field="dest_port" logic="OR" delimiter="|">4444|5555|6666|8080</node>
            <node id="known_malware_domain" type="PLUGIN">is_malware_domain(_$dest_domain)</node>
            <node id="beacon_pattern" type="PLUGIN">detect_beacon_pattern(_$connection_history)</node>
        </checklist>

        <!-- 统计不同目标IP的连接数 -->
        <threshold group_by="source_ip" range="3600s" count_type="CLASSIFY" count_field="dest_ip">10</threshold>

        <append field="threat_type">c2_communication</append>
        <append type="PLUGIN" field="threat_intelligence">get_threat_intel(_$dest_ip, _$dest_domain)</append>

        <plugin>block_connection(_$source_ip, _$dest_ip)</plugin>
        <plugin>escalate_to_soc(_$ORIDATA)</plugin>
    </rule>
</root>
```

---

## ❓ 常见问题

### XML语法错误

#### 问题：标签未闭合
```xml
<!-- 错误 -->
<rule id="test">
    <filter field="type">59</filter>
    <!-- 缺少</rule> -->

    <!-- 正确 -->
    <rule id="test">
        <filter field="type">59</filter>
    </rule>
```

#### 问题：特殊字符未处理
```xml
<!-- 错误 -->
<node type="REGEX" field="html"><script>alert('xss')</script></node>

        <!-- 正确 -->
<node type="REGEX" field="html"><![CDATA[<script>alert('xss')</script>]]></node>
```

### 属性依赖错误

#### 问题：使用condition但节点缺少id
```xml
<!-- 错误 -->
<checklist condition="a and b">
    <node type="INCL" field="exe">malware</node>
    <node type="INCL" field="path">temp</node>
</checklist>

        <!-- 正确 -->
<checklist condition="a and b">
<node id="a" type="INCL" field="exe">malware</node>
<node id="b" type="INCL" field="path">temp</node>
</checklist>
```

#### 问题：使用logic但缺少delimiter
```xml
<!-- 错误 -->
<node type="INCL" field="process" logic="OR">malware.exe|virus.exe</node>

        <!-- 正确 -->
<node type="INCL" field="process" logic="OR" delimiter="|">malware.exe|virus.exe</node>
```

### 阈值配置错误

#### 问题：SUM类型缺少count_field
```xml
<!-- 错误 -->
<threshold group_by="user_id" range="1h" count_type="SUM">1000</threshold>

        <!-- 正确 -->
<threshold group_by="user_id" range="1h" count_type="SUM" count_field="amount">1000</threshold>
```

### 性能优化建议

#### 1. 使用Filter提升性能
```xml
<!-- 好：先用filter过滤 -->
<rule id="optimized_rule">
    <filter field="event_type">process_creation</filter>
    <checklist>
        <node type="INCL" field="process_name">suspicious</node>
    </checklist>
</rule>

        <!-- 不好：没有filter -->
<rule id="slow_rule">
<checklist>
    <node type="EQU" field="event_type">process_creation</node>
    <node type="INCL" field="process_name">suspicious</node>
</checklist>
</rule>
```

#### 2. 节点类型选择
```xml
<!-- 好：按性能排序 -->
<checklist condition="null_check and string_check and regex_check">
    <node id="null_check" type="NOTNULL" field="required_field"></node>
    <node id="string_check" type="INCL" field="process_name">suspicious</node>
    <node id="regex_check" type="REGEX" field="command_line">^.*malware.*$</node>
</checklist>
```

#### 3. 合理使用阈值
```xml
<!-- 好：使用local_cache -->
<threshold group_by="source_ip" range="300s" local_cache="true">10</threshold>

        <!-- 注意：CLASSIFY类型内存消耗较大 -->
<threshold group_by="user_id" range="3600s" count_type="CLASSIFY" count_field="resource_id">100</threshold>
```