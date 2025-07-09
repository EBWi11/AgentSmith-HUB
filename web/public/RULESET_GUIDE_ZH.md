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

### 更复杂的嵌套数据示例

```xml
<root type="DETECTION" name="advanced_detection" author="your_name">
    <rule id="nested_data_rule" name="嵌套数据检测">
        <!-- 对于这样的JSON数据：{"event":{"source":{"host":"web01","type":"login"}}} -->
        <filter field="event.source.type">login</filter>

        <checklist condition="host_check and user_check">
            <!-- 检查主机名 -->
            <node id="host_check" type="EQU" field="event.source.host">web01</node>
            <!-- 检查用户信息：{"user":{"profile":{"level":"admin"}}} -->
            <node id="user_check" type="EQU" field="user.profile.level">admin</node>
        </checklist>

        <!-- 添加分析结果 -->
        <append field="detection_result">admin_login_detected</append>
    </rule>
</root>
```

**嵌套数据访问说明**：
- `field="event.source.type"` - 直接访问多层嵌套的字段
- `field="user.profile.level"` - 访问用户配置中的级别信息
- 支持任意深度的嵌套：`a.b.c.d.e...`

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

### ⚠️ 重要约束
- **每个 `<rule>` 只能包含一个 `<checklist>`**：所有检查逻辑必须在同一个 checklist 中完成
- **如需多个检查条件，请使用 `condition` 属性组合逻辑**：如 `condition="a and b"` 或 `condition="(a or b) and c"`

### 性能优化机制
- **自动排序**：系统自动按性能优化节点执行顺序
- **智能缓存**：缓存常用计算结果
- **动态线程调整**：随着规则引擎负载自动调整线程数
- **正则优化**：使用高性能正则引擎

---

## 📖 字段访问语法

### 静态值与动态值

#### 静态值（直接写入）
```xml
<!-- 固定值 -->
<node type="EQU" field="status">active</node>
<filter field="event_type">process_creation</filter>
<append field="alert_type">malware_detected</append>
```

#### 动态值（从数据中获取）
使用 `_$` 前缀从当前数据中动态获取值：
```xml
<!-- 动态值：从数据中获取 expected_status 字段的值 -->
<node type="EQU" field="status">_$expected_status</node>
<filter field="event_type">_$monitoring_target</filter>
<append field="reference_id">_$original_event_id</append>
```

### 嵌套字段访问（a.b.c语法）

#### 两种嵌套访问方式

##### 1. 在 field 属性中直接使用嵌套路径
用于访问输入数据中的深层字段：
```xml
<!-- 示例数据：{"user":{"profile":{"level":"admin"}}} -->
<node type="EQU" field="user.profile.level">admin</node>

<!-- 示例数据：{"event":{"source":{"host":"server01"}}} -->
<filter field="event.source.host">server01</filter>

<!-- 示例数据：{"request":{"headers":{"authorization":"Bearer token123"}}} -->
<node type="NOTNULL" field="request.headers.authorization"></node>

<!-- 示例数据：{"a":{"b":{"c":"test100"}}} -->
<node type="EQU" field="a.b.c">test100</node>
```

##### 2. 在值部分使用动态引用（_$前缀）
用于从数据中动态获取比较值：
```xml
<!-- 从配置中获取期望的安全级别进行比较 -->
<node type="EQU" field="user_level">_$config.expected_level</node>

<!-- 从嵌套配置中获取目标系统名称 -->
<filter field="event_type">_$monitoring.target_events</filter>

<!-- 从用户配置中获取认证令牌 -->
<node type="EQU" field="auth_token">_$user.session.token</node>
```

#### 完整对比示例

假设输入数据为：
```json
{
  "user": {
    "id": "user123",
    "profile": {
      "level": "admin",
      "department": "security"
    }
  },
  "config": {
    "min_level": "admin",
    "allowed_departments": "security,it"
  },
  "event": {
    "risk_score": 85,
    "details": {
      "source": "internal"
    }
  }
}
```

**不同的嵌套访问方式：**
```xml
<rule id="nested_access_demo" name="嵌套访问演示">
    <!-- 1. field中使用嵌套路径：直接访问输入数据 -->
    <checklist condition="level_check and score_check and source_check">
        <!-- 检查用户级别是否为admin -->
        <node id="level_check" type="EQU" field="user.profile.level">admin</node>
        
        <!-- 检查风险分数是否大于80 -->
        <node id="score_check" type="MT" field="event.risk_score">80</node>
        
        <!-- 检查事件来源 -->
        <node id="source_check" type="EQU" field="event.details.source">internal</node>
    </checklist>
    
    <!-- 2. 值中使用_$：动态获取比较值 -->
    <checklist condition="dynamic_level_check and dept_check">
        <!-- 用户级别与配置中的最小级别比较 -->
        <node id="dynamic_level_check" type="EQU" field="user.profile.level">_$config.min_level</node>
        
        <!-- 检查部门是否在允许列表中 -->
        <node id="dept_check" type="INCL" field="_$config.allowed_departments">_$user.profile.department</node>
    </checklist>
    
    <!-- 3. 在阈值和插件中使用嵌套字段 -->
    <threshold group_by="user.profile.department" range="300s">5</threshold>
    
    <append type="PLUGIN" field="user_info">get_user_details(_$user.profile.id)</append>
</rule>
```

#### 语法要点总结

| 用法 | 完整XML示例 | 说明 | 适用数据场景 |
|------|-------------|------|-------------|
| **field属性嵌套** | `<node type="EQU" field="user.profile.level">admin</node>` | 直接访问输入数据的嵌套字段，与固定值比较 | 输入数据：`{"user":{"profile":{"level":"admin"}}}` |
| **值的动态引用** | `<node type="EQU" field="status">_$config.expected_status</node>` | field访问简单字段，值从其他字段动态获取 | 输入数据：`{"status":"active", "config":{"expected_status":"active"}}` |
| **双重嵌套访问** | `<node type="EQU" field="user.profile.level">_$system.security.min_level</node>` | field访问嵌套字段，值也从嵌套字段动态获取 | 输入数据：`{"user":{"profile":{"level":"admin"}}, "system":{"security":{"min_level":"admin"}}}` |

#### 语法综合示例

假设有如下输入数据：
```json
{
  "user": {
    "id": "user123",
    "profile": {
      "level": "admin",
      "department": "security"
    }
  },
  "system": {
    "security": {
      "min_level": "admin",
      "allowed_departments": ["security", "it"]
    }
  },
  "event": {
    "type": "login",
    "timestamp": 1640995200
  }
}
```

**对应的规则写法：**
```xml
<rule id="access_control" name="访问控制检测">
    <checklist condition="level_check and dept_check and event_check">
        <!-- 1. field属性嵌套：检查用户级别是否为admin -->
        <node id="level_check" type="EQU" field="user.profile.level">admin</node>
        
        <!-- 2. 值的动态引用：用户级别与系统要求的最低级别比较 -->
        <node id="dynamic_check" type="EQU" field="user.profile.level">_$system.security.min_level</node>
        
        <!-- 3. 双重嵌套访问：事件类型与系统配置中的监控类型比较 -->
        <node id="event_check" type="EQU" field="event.type">login</node>
        
        <!-- 4. 部门权限检查：用户部门必须在允许列表中 -->
        <node id="dept_check" type="INCL" field="_$system.security.allowed_departments">_$user.profile.department</node>
    </checklist>
</rule>
```

### 原始数据访问（_$ORIDATA）

#### 什么是_$ORIDATA
`_$ORIDATA` 是一个特殊的保留字段，代表完整的原始数据对象。它包含了传入规则引擎的所有原始字段和值。

#### 使用场景
```xml
<!-- 1. 插件中传递完整数据进行复杂分析 -->
<node type="PLUGIN">complex_analysis(_$ORIDATA)</node>

<!-- 2. 在Append中使用插件处理完整数据 -->
<append type="PLUGIN" field="threat_score">calculate_threat_score(_$ORIDATA)</append>

<!-- 3. 在独立插件中发送完整数据 -->
<plugin>send_alert(_$ORIDATA, "HIGH")</plugin>
<plugin>log_security_event(_$ORIDATA)</plugin>
```

#### 实际示例
```xml
<rule id="comprehensive_analysis" name="综合分析示例">
    <filter field="event_type">security_event</filter>
    
    <checklist>
        <!-- 基础检查使用具体字段 -->
        <node type="MT" field="risk_score">_$thresholds.min_risk</node>
        <!-- 复杂分析使用完整数据 -->
        <node type="PLUGIN">deep_threat_analysis(_$ORIDATA)</node>
    </checklist>
    
    <!-- 使用嵌套字段进行分组 -->
    <threshold group_by="_$event.source.host,_$user.department" range="600s">5</threshold>
    
    <!-- 丰富化数据 -->
    <append type="PLUGIN" field="enriched_data">enrich_with_context(_$ORIDATA)</append>
    
    <!-- 发送告警 -->
    <plugin>send_comprehensive_alert(_$ORIDATA, _$analysis.priority)</plugin>
</rule>
```

### 字段访问最佳实践

#### 1. 性能优化
```xml
<!-- 好：先用简单字段过滤，再用复杂分析 -->
<rule id="optimized_rule">
    <filter field="event_type">_$config.monitored_event</filter>
    <checklist condition="basic_check and complex_check">
        <node id="basic_check" type="INCL" field="process_name">_$patterns.suspicious_process</node>
        <node id="complex_check" type="PLUGIN">analyze_full_context(_$ORIDATA)</node>
    </checklist>
</rule>
```

#### 2. 错误处理
```xml
<!-- 确保嵌套字段存在 -->
<checklist condition="field_exists and value_check">
    <node id="field_exists" type="NOTNULL" field="user.profile.id"></node>
    <node id="value_check" type="EQU" field="status">_$user.profile.expected_status</node>
</checklist>
```

#### 3. 灵活配置
```xml
<!-- 使用动态配置实现灵活的规则 -->
<rule id="configurable_rule">
    <filter field="_$config.filter_field">_$config.filter_value</filter>
    <checklist>
        <node type="_$config.check_type" field="_$config.target_field">_$config.expected_value</node>
    </checklist>
    <threshold group_by="_$config.group_fields" range="_$config.time_window">_$config.threshold_value</threshold>
</rule>
```

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
| `field` | 是  | 要过滤的字段名，**支持嵌套语法 a.b.c** | `event_type`, `user.profile.level` |

**用途**：在执行复杂检查前快速过滤数据，显著提升性能；filter 本身不是必填项

#### filter 嵌套字段示例
```xml
<!-- 简单过滤 -->
<filter field="event_type">process_creation</filter>

<!-- 嵌套过滤：过滤 {"event":{"source":{"type":"security"}}} -->
<filter field="event.source.type">security</filter>

<!-- 深层嵌套过滤 -->
<filter field="request.headers.content_type">application/json</filter>
```

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
| `field` | 条件 | 要检查的字段名（PLUGIN类型可选），**支持嵌套语法 a.b.c** | `process_name`, `user.profile.level` |
| `logic` | 否 | 多值逻辑 | `OR`, `AND` |
| `delimiter` | 条件 | 分隔符（使用logic时必需） | `|`, `,` |

#### field 字段嵌套访问示例
```xml
<!-- 简单字段 -->
<node type="EQU" field="username">admin</node>

<!-- 嵌套字段：访问 {"user":{"profile":{"level":"admin"}}} 中的 level -->
<node type="EQU" field="user.profile.level">admin</node>

<!-- 深层嵌套：访问 {"a":{"b":{"c":"test100"}}} 中的 c -->
<node type="EQU" field="a.b.c">test100</node>

<!-- 过滤器中的嵌套字段 -->
<filter field="event.source.system">web_server</filter>
```

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

### 🧩 内置插件列表

系统提供了丰富的内置插件，无需额外开发即可使用。插件分为两类：

#### 检查节点插件（CheckNode）
用于条件判断，返回布尔值，可在 `<node type="PLUGIN">` 中使用：

| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `isPrivateIP` | 检查IP是否为私有地址 | `ip` (string) | `<node type="PLUGIN">isPrivateIP(source_ip)</node>` |
| `cidrMatch` | 检查IP是否在CIDR范围内 | `ip` (string), `cidr` (string) | `<node type="PLUGIN">cidrMatch(client_ip, "192.168.1.0/24")</node>` |
| `geoMatch` | 检查IP地理位置是否匹配 | `ip` (string), `countryISO` (string) | `<node type="PLUGIN">geoMatch(source_ip, "US")</node>` |
| `suppressOnce` | 告警抑制：时间窗口内只触发一次 | `key` (any), `windowSec` (int), `ruleid` (string, 可选) | `<node type="PLUGIN">suppressOnce(alert_key, 300, "rule_001")</node>` |

#### 数据处理插件（Append）
用于数据转换和丰富化，可在 `<append type="PLUGIN">` 中使用：

##### 时间处理插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `now` | 获取当前时间戳 | 可选: `format` (unix/ms/rfc3339) | `<append type="PLUGIN" field="timestamp">now()</append>` |
| `ago` | 获取N秒前的时间戳 | `seconds` (int/float/string) | `<append type="PLUGIN" field="past_time">ago(3600)</append>` |
| `dayOfWeek` | 获取星期几(0-6, 0=周日) | 可选: `timestamp` (int64) | `<append type="PLUGIN" field="weekday">dayOfWeek()</append>` |
| `hourOfDay` | 获取小时(0-23) | 可选: `timestamp` (int64) | `<append type="PLUGIN" field="hour">hourOfDay()</append>` |
| `tsToDate` | 时间戳转RFC3339格式 | `timestamp` (int64) | `<append type="PLUGIN" field="formatted_time">tsToDate(event_time)</append>` |

##### 编码和哈希插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `base64Encode` | Base64编码 | `input` (string) | `<append type="PLUGIN" field="encoded">base64Encode(raw_data)</append>` |
| `base64Decode` | Base64解码 | `encoded` (string) | `<append type="PLUGIN" field="decoded">base64Decode(encoded_data)</append>` |
| `hashMD5` | 计算MD5哈希 | `input` (string) | `<append type="PLUGIN" field="md5">hashMD5(password)</append>` |
| `hashSHA1` | 计算SHA1哈希 | `input` (string) | `<append type="PLUGIN" field="sha1">hashSHA1(content)</append>` |
| `hashSHA256` | 计算SHA256哈希 | `input` (string) | `<append type="PLUGIN" field="sha256">hashSHA256(file_data)</append>` |

##### URL处理插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `extractDomain` | 从URL提取域名 | `urlOrHost` (string) | `<append type="PLUGIN" field="domain">extractDomain(request_url)</append>` |
| `extractTLD` | 从域名提取顶级域名 | `domain` (string) | `<append type="PLUGIN" field="tld">extractTLD(hostname)</append>` |
| `extractSubdomain` | 从主机名提取子域名 | `host` (string) | `<append type="PLUGIN" field="subdomain">extractSubdomain(full_hostname)</append>` |

##### 字符串处理插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `replace` | 字符串替换 | `input` (string), `old` (string), `new` (string) | `<append type="PLUGIN" field="cleaned">replace(raw_text, "bad", "good")</append>` |
| `regexExtract` | 正则表达式提取 | `input` (string), `pattern` (string) | `<append type="PLUGIN" field="extracted">regexExtract(log_line, "IP: (\\d+\\.\\d+\\.\\d+\\.\\d+)")</append>` |
| `regexReplace` | 正则表达式替换 | `input` (string), `pattern` (string), `replacement` (string) | `<append type="PLUGIN" field="masked">regexReplace(email, "(.+)@(.+)", "$1@***")</append>` |

##### 数据解析插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `parseJSON` | 解析JSON字符串 | `jsonString` (string) | `<append type="PLUGIN" field="parsed">parseJSON(json_data)</append>` |
| `parseUA` | 解析User-Agent | `userAgent` (string) | `<append type="PLUGIN" field="browser_info">parseUA(user_agent)</append>` |

##### 威胁情报插件
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `virusTotal` | 查询VirusTotal文件哈希威胁情报 | `hash` (string), `apiKey` (string, 可选) | `<append type="PLUGIN" field="vt_scan">virusTotal(file_hash)</append>` |
| `shodan` | 查询Shodan IP地址基础设施情报 | `ip` (string), `apiKey` (string, 可选) | `<append type="PLUGIN" field="shodan_intel">shodan(ip_address)</append>` |
| `threatBook` | 查询微步在线威胁情报 | `queryValue` (string), `queryType` (string), `apiKey` (string, 可选) | `<append type="PLUGIN" field="tb_intel">threatBook(target_ip, "ip")</append>` |

### 内置插件使用示例

#### 1. 网络安全检测
```xml
<rule id="network_security" name="网络安全检测">
    <filter field="event_type">network_connection</filter>
    
    <checklist condition="(external_conn and (suspicious_geo or private_ip_abuse)) and suppress_check">
        <!-- 检查是否为外部连接 -->
        <node id="external_conn" type="PLUGIN">isPrivateIP(dest_ip)</node>
        <!-- 检查地理位置 -->
        <node id="suspicious_geo" type="PLUGIN">geoMatch(source_ip, "CN")</node>
        <!-- 检查源IP是否在可疑网段 -->
        <node id="private_ip_abuse" type="PLUGIN">cidrMatch(source_ip, "10.0.0.0/8")</node>
        <!-- 告警抑制：同一IP 5分钟内只告警一次（使用ruleid隔离不同规则） -->
        <node id="suppress_check" type="PLUGIN">suppressOnce(source_ip, 300, "network_security")</node>
    </checklist>
    
    <!-- 数据丰富化 -->
    <append type="PLUGIN" field="source_domain">extractDomain(source_url)</append>
    <append type="PLUGIN" field="detection_time">now()</append>
    <append type="PLUGIN" field="url_hash">hashSHA256(request_url)</append>
</rule>
```

#### 2. 日志分析和处理
```xml
<rule id="log_analysis" name="日志分析处理">
    <filter field="event_type">application_log</filter>
    
    <checklist>
        <!-- 检查是否包含JSON数据 -->
        <node type="INCL" field="log_message">{"</node>
    </checklist>
    
    <!-- 解析和丰富化 -->
    <append type="PLUGIN" field="parsed_log">parseJSON(log_message)</append>
    <append type="PLUGIN" field="log_hour">hourOfDay()</append>
    <append type="PLUGIN" field="log_weekday">dayOfWeek()</append>
    <append type="PLUGIN" field="user_agent_info">parseUA(user_agent)</append>
    
    <!-- 数据清理 -->
    <append type="PLUGIN" field="cleaned_path">regexReplace(request_path, "/\\d+", "/ID")</append>
    <append type="PLUGIN" field="masked_email">regexReplace(email, "(.{2}).*@(.+)", "$1***@$2")</append>
</rule>
```

#### 3. 时间窗口分析
```xml
<rule id="time_window_analysis" name="时间窗口分析">
    <filter field="event_type">user_activity</filter>
    
    <!-- 数据预处理 -->
    <append type="PLUGIN" field="one_hour_ago">ago(3600)</append>
    <append type="PLUGIN" field="activity_hour">hourOfDay(activity_timestamp)</append>
    
    <!-- 时间范围检查 -->
    <checklist condition="work_hours and recent_activity">
        <node id="work_hours" type="MT" field="activity_hour">8</node>
        <node id="recent_activity" type="MT" field="activity_timestamp">_$one_hour_ago</node>
    </checklist>
    
    <!-- 生成报告时间 -->
    <append type="PLUGIN" field="report_time">tsToDate(activity_timestamp)</append>
</rule>
```

#### 4. 数据脱敏和安全处理
```xml
<rule id="data_masking" name="数据脱敏处理">
    <filter field="contains_sensitive_data">true</filter>
    
    <!-- 数据哈希化 -->
    <append type="PLUGIN" field="user_id_hash">hashSHA256(user_id)</append>
    <append type="PLUGIN" field="session_hash">hashMD5(session_id)</append>
    
    <!-- 敏感信息编码 -->
    <append type="PLUGIN" field="encoded_payload">base64Encode(sensitive_payload)</append>
    
    <!-- 清理和替换 -->
    <append type="PLUGIN" field="cleaned_log">replace(raw_log, user_password, "***")</append>
    <append type="PLUGIN" field="masked_phone">regexReplace(phone_number, "(\\d{3})\\d{4}(\\d{4})", "$1****$2")</append>
    
    <!-- 删除原始敏感数据 -->
    <del>user_password,raw_sensitive_data,unencrypted_payload</del>
</rule>
```

#### 5. 威胁情报分析
```xml
<rule id="threat_intelligence" name="威胁情报分析">
    <filter field="event_type">security_event</filter>
    
    <checklist condition="ip_check and (file_check or url_check or domain_check)">
        <!-- 检查是否有IP地址 -->
        <node id="ip_check" type="NOTNULL" field="source_ip"></node>
        <!-- 检查是否有文件哈希 -->
        <node id="file_check" type="NOTNULL" field="file_hash"></node>
        <!-- 检查是否有URL -->
        <node id="url_check" type="NOTNULL" field="suspicious_url"></node>
        <!-- 检查是否有域名 -->
        <node id="domain_check" type="NOTNULL" field="domain"></node>
    </checklist>
    
    <!-- 威胁情报丰富化 -->
    <append type="PLUGIN" field="shodan_intel">shodan(source_ip)</append>
    <append type="PLUGIN" field="virustotal_scan">virusTotal(file_hash)</append>
    <append type="PLUGIN" field="threatbook_ip">threatBook(source_ip, "ip")</append>
    <append type="PLUGIN" field="threatbook_file">threatBook(file_hash, "file", "api_key")</append>
    <append type="PLUGIN" field="threatbook_domain">threatBook(domain, "domain", "api_key")</append>
    <append type="PLUGIN" field="threatbook_url">threatBook(suspicious_url, "url")</append>
    
    <!-- 综合威胁评分 -->
    <append type="PLUGIN" field="threat_score">calculate_threat_score(_$ORIDATA)</append>
    <append type="PLUGIN" field="analysis_time">now()</append>
</rule>
```

### ⚠️ 告警抑制最佳实践（suppressOnce）

#### 为什么需要 ruleid 参数？

**问题示例**：
```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <checklist>
        <node type="PLUGIN">suppressOnce(source_ip, 300)</node>
    </checklist>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <checklist>
        <node type="PLUGIN">suppressOnce(source_ip, 300)</node>
    </checklist>
</rule>
```

**问题**：规则A触发后，规则B对同一IP也会被抑制！

#### 正确用法

**解决方案**：使用 `ruleid` 参数隔离不同规则：
```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <checklist>
        <node type="PLUGIN">suppressOnce(source_ip, 300, "network_threat")</node>
    </checklist>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <checklist>
        <node type="PLUGIN">suppressOnce(source_ip, 300, "login_anomaly")</node>
    </checklist>
</rule>
```

#### Redis Key 结构
- **不带 ruleid**：`suppress_once:192.168.1.100`
- **带 ruleid**：`suppress_once:network_threat:192.168.1.100`

这样不同规则的抑制机制完全独立！

#### 推荐命名规范
- 使用规则ID作为 ruleid：`suppressOnce(key, window, "rule_id")`
- 或使用业务标识：`suppressOnce(key, window, "login_brute_force")`

### 插件性能说明

#### 性能等级（从高到低）：
1. **检查节点插件**：`isPrivateIP`, `cidrMatch` - 纯计算，性能较高
2. **字符串处理插件**：`replace`, `hashMD5/SHA1/SHA256` - 中等性能
3. **正则表达式插件**：`regexExtract`, `regexReplace` - 性能较低
4. **数据库查询插件**：`geoMatch` - 需要数据库查询，性能较低
5. **威胁情报插件**：`virusTotal`, `shodan`, `threatBook` - 外部API调用，性能最低

#### 优化建议：
```xml
<!-- 好：先用高性能检查，再用低性能插件 -->
<checklist condition="basic_check and geo_check">
    <node id="basic_check" type="PLUGIN">isPrivateIP(source_ip)</node>
    <node id="geo_check" type="PLUGIN">geoMatch(source_ip, "US")</node>
</checklist>

<!-- 避免：在大量数据上频繁使用低性能插件 -->
<checklist>
    <node type="PLUGIN">geoMatch(source_ip, "US")</node>
</checklist>

<!-- 威胁情报插件优化：利用缓存和条件判断 -->
<rule id="threat_intel_optimized">
    <filter field="event_type">security_event</filter>
    
    <checklist condition="has_suspicious_indicators and need_enrichment">
        <!-- 先用高性能检查确认需要查询 -->
        <node id="has_suspicious_indicators" type="INCL" field="alert_level">high</node>
        <node id="need_enrichment" type="NOTNULL" field="source_ip"></node>
    </checklist>
    
    <!-- 然后才使用威胁情报插件 -->
    <append type="PLUGIN" field="threat_intel">threatBook(source_ip, "ip")</append>
</rule>
```

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

插件调用支持多种参数类型，请参考前面的"字段访问语法"章节了解详细用法。

#### 参数类型概述
- **字面量参数**：直接写入固定值，如 `"high"`, `100`, `true`
- **字段引用参数**：直接引用数据中的字段，如 `user_id`, `session_token`
- **动态字段参数**：使用 `_$` 前缀引用字段，如 `_$user.profile.id`
- **原始数据参数**：使用 `_$ORIDATA` 传递完整数据

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
<rule id="optimized_rule">
    <checklist condition="basic_check and plugin_check">
        <node id="basic_check" type="INCL" field="process_name">suspicious</node>
        <node id="plugin_check" type="PLUGIN">deep_analysis(_$ORIDATA)</node>
    </checklist>
</rule>

<!-- 不好：直接使用插件 -->
<rule id="slow_rule">
    <checklist>
        <node type="PLUGIN">complex_analysis(_$ORIDATA)</node>
    </checklist>
</rule>
```

#### 2. 错误处理
```xml
<rule id="safe_rule">
    <!-- 插件应该优雅处理错误 -->
    <checklist condition="safe_check and plugin_check">
        <node id="safe_check" type="NOTNULL" field="required_field"></node>
        <node id="plugin_check" type="PLUGIN">safe_analysis(_$required_field)</node>
    </checklist>
</rule>
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
<rule id="and_logic_rule">
    <checklist>
        <node type="INCL" field="process">malware</node>
        <node type="INCL" field="path">temp</node>
    </checklist>
</rule>

<!-- OR逻辑 -->
<rule id="or_logic_rule">
    <checklist condition="a or b">
        <node id="a" type="INCL" field="process">malware</node>
        <node id="b" type="INCL" field="path">suspicious</node>
    </checklist>
</rule>
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

### 案例5：综合威胁情报分析

```xml
<root type="DETECTION" name="comprehensive_threat_intel" author="security_team">
    <rule id="multi_source_threat_analysis" name="多源威胁情报分析">
        <filter field="event_type">security_alert</filter>

        <checklist condition="has_indicators and (high_risk or multiple_sources)">
            <node id="has_indicators" type="NOTNULL" field="threat_indicator"></node>
            <node id="high_risk" type="INCL" field="alert_level">high</node>
            <node id="multiple_sources" type="INCL" field="source_count">3</node>
        </checklist>

        <!-- 10分钟内同一威胁指标不重复分析 -->
        <threshold group_by="threat_indicator" range="600s" local_cache="true">1</threshold>

        <!-- 多源威胁情报查询 -->
        <append type="PLUGIN" field="virustotal_intel">virusTotal(file_hash)</append>
        <append type="PLUGIN" field="shodan_intel">shodan(source_ip)</append>
        <append type="PLUGIN" field="threatbook_ip">threatBook(source_ip, "ip", "prod_api_key")</append>
        <append type="PLUGIN" field="threatbook_domain">threatBook(domain, "domain", "prod_api_key")</append>
        <append type="PLUGIN" field="threatbook_file">threatBook(file_hash, "file", "prod_api_key")</append>
        <append type="PLUGIN" field="threatbook_url">threatBook(suspicious_url, "url", "prod_api_key")</append>

        <!-- 综合分析 -->
        <append type="PLUGIN" field="threat_score">calculate_comprehensive_threat_score(_$ORIDATA)</append>
        <append type="PLUGIN" field="malware_family">identify_malware_family(_$ORIDATA)</append>
        <append type="PLUGIN" field="attack_timeline">construct_attack_timeline(_$ORIDATA)</append>
        <append type="PLUGIN" field="ioc_correlation">correlate_iocs(_$ORIDATA)</append>

        <!-- 分析结果 -->
        <append field="analysis_type">comprehensive_threat_intelligence</append>
        <append type="PLUGIN" field="analysis_timestamp">now()</append>
        <append type="PLUGIN" field="analyst_recommendations">generate_recommendations(_$threat_score, _$malware_family)</append>

        <!-- 自动化响应 -->
        <plugin>enrich_threat_database(_$ORIDATA)</plugin>
        <plugin>trigger_automated_response(_$threat_score, _$analyst_recommendations)</plugin>
        <plugin>notify_threat_intel_team(_$ORIDATA)</plugin>
    </rule>

    <rule id="chinese_threat_analysis" name="中文威胁情报分析">
        <filter field="event_type">apt_activity</filter>

        <checklist condition="chinese_context and needs_local_intel">
            <node id="chinese_context" type="INCL" field="geo_location" logic="OR" delimiter="|">CN|HK|TW|SG</node>
            <node id="needs_local_intel" type="INCL" field="threat_category">apt</node>
        </checklist>

        <!-- 使用微步在线进行中文威胁情报分析 -->
        <append type="PLUGIN" field="threatbook_comprehensive">threatBook(threat_indicator, indicator_type, "china_api_key")</append>
        <append type="PLUGIN" field="chinese_malware_family">identify_chinese_malware(_$threatbook_comprehensive)</append>
        <append type="PLUGIN" field="apt_group_attribution">attribute_apt_group(_$threatbook_comprehensive)</append>

        <!-- 结合其他情报源 -->
        <append type="PLUGIN" field="global_context">combine_global_local_intel(_$threatbook_comprehensive, _$virustotal_intel)</append>

        <!-- 生成中文威胁报告 -->
        <append type="PLUGIN" field="chinese_threat_report">generate_chinese_report(_$ORIDATA)</append>
        <append field="report_language">zh-CN</append>

        <plugin>alert_chinese_security_team(_$ORIDATA)</plugin>
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

### 规则结构错误

#### 问题：在同一个rule中使用多个checklist
```xml
<!-- 错误：一个rule中有多个checklist -->
<rule id="wrong_rule">
    <checklist>
        <node type="INCL" field="process">malware</node>
    </checklist>
    <checklist>
        <node type="INCL" field="path">temp</node>
    </checklist>
</rule>

<!-- 正确：一个rule只有一个checklist -->
<rule id="correct_rule">
    <checklist condition="malware_check and path_check">
        <node id="malware_check" type="INCL" field="process">malware</node>
        <node id="path_check" type="INCL" field="path">temp</node>
    </checklist>
</rule>
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