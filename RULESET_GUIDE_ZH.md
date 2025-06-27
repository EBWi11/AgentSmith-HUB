# 🛡️ AgentSmith-HUB 规则编辑完整指南

## 📋 目录
1. [基础概念](#基础概念)
2. [快速参考](#快速参考)
3. [Step by Step 快速入门](#step-by-step-快速入门)
4. [完整语法参考](#完整语法参考)
5. [最佳实践](#最佳实践)
6. [示例规则集](#示例规则集)

---

## 🧠 基础概念

### 核心组件
- **规则集 (Ruleset)**: 一组相关规则的集合，类型为 `DETECTION` 或 `WHITELIST`
- **规则 (Rule)**: 单个检测逻辑单元，包含过滤器、检查列表、阈值等
- **检查节点 (Node)**: 具体的检查操作，支持23种检查类型

### 数据字段访问
- **静态值**: 直接指定字符串或数字
- **动态值**: 使用 `_$` 前缀从原始数据中获取字段值
- **示例**: `_$exe` 表示获取数据中的 `exe` 字段值

---

## 📋 快速参考

### 元素属性必需性速查表

| 元素 | 属性 | 必需性 | 说明 |
|------|------|--------|------|
| `<root>` | `type` | **必需** | DETECTION 或 WHITELIST |
| | `name` | *可选* | 规则集名称 |
| | `author` | *可选* | 作者信息 |
| `<rule>` | `id` | **必需** | 唯一标识符 |
| | `name` | *可选* | 显示名称 |
| `<filter>` | `field` | *可选* | 字段名 |
| `<checklist>` | `condition` | *可选* | 逻辑条件表达式 |
| └─ `<node>` | `id` | *高级功能* | 使用condition时**必需** |
| | `type` | **必需** | 检查类型 |
| | `field` | *类型依赖* | PLUGIN类型时可选 |
| | `logic` | *可选* | AND/OR逻辑 |
| | `delimiter` | *依赖logic* | 有logic时必需 |
| `<threshold>` | `group_by` | **必需** | 分组字段 |
| | `range` | **必需** | 时间窗口 |
| | `count_type` | *可选* | 计数类型 |
| | `count_field` | *依赖count_type* | SUM/CLASSIFY时必需 |
| | `local_cache` | *可选* | 本地缓存开关 |
| `<append>` | `field` | **必需** | 字段名 |
| | `type` | *可选* | 仅支持PLUGIN |
| `<plugin>` | - | - | 仅内容必需 |
| `<del>` | - | - | 仅内容必需 |

### 检查类型速查表

| 类型 | 功能 | 示例 |
|------|------|------|
| **字符串检查** |
| `INCL` | 包含 | `<node type="INCL" field="exe">malware</node>` |
| `NI` | 不包含 | `<node type="NI" field="process">trusted</node>` |
| `START` | 开头匹配 | `<node type="START" field="path">C:\</node>` |
| `END` | 结尾匹配 | `<node type="END" field="file">.exe</node>` |
| `NSTART` | 不以开头 | `<node type="NSTART" field="path">C:\Windows</node>` |
| `NEND` | 不以结尾 | `<node type="NEND" field="file">.tmp</node>` |
| **相等检查** |
| `EQU` | 相等 | `<node type="EQU" field="status">active</node>` |
| `NEQ` | 不相等 | `<node type="NEQ" field="user">guest</node>` |
| **大小写忽略** |
| `NCS_*` | 忽略大小写版本 | 对应上述类型的忽略大小写版本 |
| **数值比较** |
| `MT` | 大于 | `<node type="MT" field="score">75</node>` |
| `LT` | 小于 | `<node type="LT" field="usage">90</node>` |
| **空值检查** |
| `ISNULL` | 为空 | `<node type="ISNULL" field="optional"></node>` |
| `NOTNULL` | 非空 | `<node type="NOTNULL" field="required"></node>` |
| **高级检查** |
| `REGEX` | 正则 | `<node type="REGEX" field="ip">^192\.168\.</node>` |
| | | 复杂正则: `<node type="REGEX"><![CDATA[<tag>]]></node>` |
| `PLUGIN` | 插件 | `<node type="PLUGIN">is_malicious(_$domain)</node>` |

> **⚠️ 重要提示**: 当任何元素内容（filter值、node值、append值、plugin参数等）包含XML特殊字符（`<`、`>`、`&`、`"`、`'`）时，必须使用 `<![CDATA[...]]>` 包裹内容，否则会导致XML解析错误。

---

## 🚀 Step by Step 快速入门

### 第一步：创建基本规则集
```xml
<root type="DETECTION" name="my_rules" author="your_name">
    <!-- 规则将在这里定义 -->
</root>
```

### 第二步：添加基础规则
```xml
<root type="DETECTION">
    <rule id="basic_rule_01" name="基础检测规则">
        <filter field="data_type">59</filter>
        <checklist>
            <node type="INCL" field="exe">suspicious_process</node>
        </checklist>
    </rule>
</root>
```

### 第三步：使用复杂条件逻辑
```xml
<rule id="complex_rule_01" name="复杂条件检测">
    <filter field="data_type">59</filter>
    <checklist condition="a and (b or c)">
        <node id="a" type="REGEX" field="exe">.*malware.*</node>
        <node id="b" type="INCL" field="argv">--backdoor</node>
        <node id="c" type="START" field="cmdline">powershell</node>
    </checklist>
</rule>

<!-- 使用CDATA的复杂正则示例 -->
<rule id="html_injection_01" name="HTML注入检测">
    <filter field="event_type">web_request</filter>
    <checklist>
        <node type="REGEX" field="request_data"><![CDATA[<.*?script.*?>|<.*?iframe.*?>]]></node>
    </checklist>
</rule>
```

### 第四步：添加阈值检测
```xml
<rule id="threshold_rule_01" name="频率阈值检测">
    <filter field="data_type">59</filter>
    <checklist>
        <node type="INCL" field="exe">suspicious_app</node>
    </checklist>
    <threshold group_by="exe,source_ip" range="5m" count_type="SUM" count_field="count" local_cache="true">10</threshold>
</rule>
```

### 第五步：添加字段操作
```xml
<rule id="append_rule_01" name="字段追加规则">
    <filter field="data_type">59</filter>
    <checklist>
        <node type="INCL" field="exe">target_process</node>
    </checklist>
    <append field="alert_level">HIGH</append>
    <append field="process_name">_$exe</append>
    <del>unnecessary_field1,unnecessary_field2</del>
</rule>
```

### 第六步：集成插件功能
```xml
<rule id="plugin_rule_01" name="插件集成规则">
    <filter field="data_type">59</filter>
    <checklist>
        <node type="PLUGIN">is_local_ip(_$source_ip)</node>
    </checklist>
    <append type="PLUGIN" field="geo_info">get_ip_location(_$source_ip)</append>
    <plugin>send_alert(_$ORIDATA, "HIGH", "Detected suspicious activity")</plugin>
</rule>
```

---

## 📚 完整语法参考

### 根元素 (root)
```xml
<root type="DETECTION|WHITELIST" name="ruleset_name" author="author_name">
    <!-- 规则定义 -->
</root>
```

**属性说明**:
- `type`: **必需** - 规则集类型，`DETECTION` 或 `WHITELIST`
- `name`: *可选* - 规则集名称
- `author`: *可选* - 作者信息

### 规则定义 (rule)
```xml
<rule id="unique_rule_id" name="display_name">
    <filter field="field_name">filter_value</filter>          <!-- 可选元素 -->
    <checklist condition="logical_expression">                 <!-- 可选元素 -->
        <!-- 检查节点 -->
    </checklist>
    <threshold>threshold_value</threshold>                     <!-- 可选元素 -->
    <append field="new_field">value</append>                  <!-- 可选元素，可多个 -->
    <plugin>plugin_call</plugin>                              <!-- 可选元素，可多个 -->
    <del>field1,field2</del>                                  <!-- 可选元素 -->
</rule>
```

**属性说明**:
- `id`: **必需** - 唯一规则标识符
- `name`: *可选* - 规则显示名称

**子元素说明**:
- `<filter>`: *可选* - 预过滤条件，强烈建议使用以提高性能（无filter时所有数据都会进入检查）
- `<checklist>`: *可选* - 主要检查逻辑，不存在时规则总是匹配
- `<threshold>`: *可选* - 阈值检测配置
- `<append>`: *可选* - 字段追加操作，可以有多个
- `<plugin>`: *可选* - 插件执行，可以有多个
- `<del>`: *可选* - 字段删除操作

### 过滤器 (filter)
```xml
<filter field="field_name">filter_value</filter>
```

**属性说明**:
- `field`: *可选* - 要检查的字段名（为空时跳过过滤）
- **元素内容**: *依赖field* - 过滤值（当field存在时**必需**，支持静态值或`_$`动态值）

### 检查列表 (checklist)
```xml
<checklist condition="logical_expression">
    <!-- 检查节点 -->
</checklist>
```

**属性说明**:
- `condition`: *可选* - 高级逻辑表达式（不指定时默认为AND逻辑，指定时可使用复杂逻辑组合）
- **子元素**: **必需** - 至少包含一个`<node>`元素
- **默认逻辑**: 无condition时，所有node必须都通过（AND逻辑）

### 检查节点 (node)
> **注意**: `<node>` 元素是 `<checklist>` 的子元素，只能在 `<checklist>` 内部使用

```xml
<!-- 简单AND逻辑（默认）：所有node都必须通过 -->
<checklist>
    <node type="INCL" field="exe">malware</node>
    <node type="INCL" field="path">suspicious</node>
</checklist>

<!-- 高级逻辑：使用condition自定义逻辑组合 -->
<checklist condition="a and (b or c)">
    <node id="a" type="INCL" field="exe">malware</node>
    <node id="b" type="INCL" field="path">temp</node>
    <node id="c" type="INCL" field="path">downloads</node>
</checklist>
```

**属性说明**:
- `id`: *高级功能* - 节点标识符（使用condition高级逻辑时**必需**，简单AND逻辑时*可选*）
- `type`: **必需** - 检查类型，支持23种类型
- `field`: *类型依赖* - 字段名（PLUGIN类型时*可选*，其他类型**必需**）
- `logic`: *可选* - 多值逻辑操作（AND/OR，与delimiter配合使用）
- `delimiter`: *依赖logic* - 多值分隔符（当指定logic时**必需**）
- **元素内容**: **必需** - 检查值或插件调用表达式
  - **CDATA使用**: 任何含XML特殊字符(`<>&"'`)的内容都需要用`<![CDATA[...]]>`包裹

### 检查节点完整类型列表

#### 字符串检查
- **INCL**: 包含检查 `<node type="INCL" field="exe">malware</node>`
- **NI**: 不包含检查 `<node type="NI" field="process">trusted</node>`
- **START**: 开头匹配 `<node type="START" field="cmdline">powershell</node>`
- **END**: 结尾匹配 `<node type="END" field="filename">.exe</node>`
- **NSTART**: 不以开头 `<node type="NSTART" field="path">C:\Windows\</node>`
- **NEND**: 不以结尾 `<node type="NEND" field="filename">.tmp</node>`

#### 相等检查
- **EQU**: 相等比较 `<node type="EQU" field="status">active</node>`
- **NEQ**: 不相等比较 `<node type="NEQ" field="user">guest</node>`

#### 忽略大小写检查
- **NCS_INCL**: 忽略大小写包含
- **NCS_NI**: 忽略大小写不包含
- **NCS_START**: 忽略大小写开头匹配
- **NCS_END**: 忽略大小写结尾匹配
- **NCS_NSTART**: 忽略大小写不以开头
- **NCS_NEND**: 忽略大小写不以结尾
- **NCS_EQU**: 忽略大小写相等
- **NCS_NEQ**: 忽略大小写不相等

#### 数值比较
- **MT**: 大于比较 `<node type="MT" field="score">75</node>`
- **LT**: 小于比较 `<node type="LT" field="cpu_usage">90</node>`

#### 空值检查
- **ISNULL**: 空值检查 `<node type="ISNULL" field="optional_field"></node>`
- **NOTNULL**: 非空检查 `<node type="NOTNULL" field="required_field"></node>`

#### 正则表达式
- **REGEX**: 正则匹配
  ```xml
  <!-- 简单正则（无特殊字符） -->
  <node type="REGEX" field="ip">^192\.168\.\d+\.\d+$</node>
  
  <!-- 复杂正则（含XML特殊字符，需要CDATA包裹） -->
  <node type="REGEX" field="html_content"><![CDATA[<script[^>]*>.*?</script>]]></node>
  ```
  
  **CDATA使用时机判断**：
  - ✅ 需要CDATA：含有 `<`、`>`、`&`、`"`、`'` 字符的正则或内容
  - ❌ 无需CDATA：仅含字母、数字、`.`、`*`、`+`、`?`、`[]`、`()`、`\` 的简单正则

#### 插件调用
- **PLUGIN**: 插件函数 `<node type="PLUGIN">is_malicious(_$domain)</node>`

### 多值检查语法
```xml
<!-- OR 逻辑：任一值匹配即可 -->
<node type="INCL" field="process" logic="OR" delimiter="|">malware.exe|virus.exe|trojan.exe</node>

<!-- AND 逻辑：所有值都必须匹配 -->
<node type="INCL" field="cmdline" logic="AND" delimiter="|">--execute|--payload|--hidden</node>
```

### 阈值检测 (threshold)
```xml
<threshold group_by="field1,field2" range="5m" count_type="SUM" count_field="amount" local_cache="true">10</threshold>
```

**属性说明**：
- `group_by`: **必需** - 分组字段，逗号分隔多个字段
- `range`: **必需** - 时间窗口（支持s/m/h/d单位，如：30s, 5m, 1h, 1d）
- `count_type`: *可选* - 计数类型：
  - *留空*: 简单计数（默认）
  - `SUM`: 对count_field字段求和
  - `CLASSIFY`: 对count_field字段去重计数
- `count_field`: *依赖count_type* - 计数字段（当count_type为SUM或CLASSIFY时**必需**）
- `local_cache`: *可选* - 是否使用本地缓存（true/false，推荐true提高性能）
- **元素内容**: **必需** - 阈值数值（超过此值时触发规则）

### 字段追加 (append)
```xml
<append field="new_field_name" type="PLUGIN">value_or_plugin_call</append>
```

**属性说明**：
- `field`: **必需** - 新字段名称
- `type`: *可选* - 追加类型（仅支持`PLUGIN`，用于插件动态值）
- **元素内容**: **必需** - 字段值（静态值、`_$`动态值或插件调用）

**使用示例**：
```xml
<!-- 静态值追加 -->
<append field="alert_level">HIGH</append>

<!-- 动态值追加 -->
<append field="original_exe">_$exe</append>

<!-- 插件值追加 -->
<append type="PLUGIN" field="geo_info">get_location(_$ip)</append>
```

### 插件执行 (plugin)
```xml
<plugin>plugin_function_call(_$ORIDATA, "param1", param2)</plugin>
```

**属性说明**：
- **元素内容**: **必需** - 插件函数调用表达式
- **特殊参数**: `_$ORIDATA` 表示传递完整原始数据

### 字段删除 (del)
```xml
<del>field1,field2,field3</del>
```

**属性说明**：
- **元素内容**: **必需** - 要删除的字段名列表，逗号分隔

---

## 🎯 最佳实践

### 1. 规则设计原则
- 使用描述性的ID和名称
- 先用高选择性字段进行预过滤
- 合理设计条件逻辑层次

### 2. 性能优化
```xml
<!-- 推荐：使用本地缓存 -->
<threshold group_by="source_ip" range="5m" local_cache="true">10</threshold>

<!-- 推荐：高效的预过滤 -->
<filter field="event_type">process_creation</filter>
```

### 3. 可维护性
```xml
<!-- 清晰的节点命名 -->
<checklist condition="suspicious_process and network_activity">
    <node id="suspicious_process" type="INCL" field="exe">malware</node>
    <node id="network_activity" type="NOTNULL" field="remote_ip"></node>
</checklist>
```

### 4. 错误避免
- 确保XML语法正确（标签闭合、属性引号）
- 条件表达式中的ID必须在节点中定义
- 动态字段引用时确保字段名正确
- **XML特殊字符注意事项**：
  - 任何元素内容含XML特殊字符(`<>&"'`)时必须使用CDATA包裹
  - 适用范围：filter值、node值、append值、plugin参数等
  - 简单内容（仅字母数字和常见符号）可直接书写
  - 复杂内容（含`<>&"'`等）必须用`<![CDATA[内容]]>`包裹
- **必需属性检查**：
  - rule元素必须有id属性
  - checklist中的node元素必须有type属性
  - node的id属性：使用condition高级逻辑时必需，简单AND逻辑时可选
  - filter元素的field属性可选（为空时跳过过滤，建议填写以提高性能）
  - threshold元素必须有group_by和range属性
  - 当count_type为SUM或CLASSIFY时必须指定count_field
  - 当logic属性存在时必须指定delimiter属性

---

## 📝 示例规则集

### 恶意PowerShell检测
```xml
<root type="DETECTION" name="powershell_detection">
    <rule id="malicious_powershell_001" name="恶意PowerShell执行">
        <filter field="data_type">59</filter>
        <checklist condition="powershell_proc and (encoded_cmd or bypass_policy)">
            <node id="powershell_proc" type="INCL" field="exe">powershell</node>
            <node id="encoded_cmd" type="INCL" field="cmdline">-EncodedCommand</node>
            <node id="bypass_policy" type="INCL" field="cmdline">-ExecutionPolicy Bypass</node>
        </checklist>
        <threshold group_by="source_ip" range="10m" local_cache="true">3</threshold>
        <append field="alert_type">malicious_powershell</append>
        <append field="severity">high</append>
        <plugin>send_alert(_$ORIDATA, "HIGH", "Malicious PowerShell detected")</plugin>
        <del>raw_data</del>
    </rule>
    
    <!-- 使用CDATA的复杂正则示例 -->
    <rule id="script_injection_001" name="脚本注入检测">
        <filter field="data_type">web_request</filter>
        <checklist>
            <node type="REGEX" field="request_body"><![CDATA[<script[^>]*>.*?</script>|javascript:.*?|on\w+\s*=]]></node>
        </checklist>
        <append field="attack_type">script_injection</append>
    </rule>
</root>
```

### 可疑网络连接检测
```xml
<root type="DETECTION" name="network_detection">
    <rule id="suspicious_network_001" name="可疑网络连接">
        <filter field="data_type">42</filter>
        <checklist condition="external_ip and high_risk_port">
            <node id="external_ip" type="PLUGIN">is_external_ip(_$dest_ip)</node>
            <node id="high_risk_port" type="INCL" field="dest_port" logic="OR" delimiter="|">4444|5555|6666|8080</node>
        </checklist>
        <threshold group_by="source_ip" range="5m" count_type="CLASSIFY" count_field="dest_ip" local_cache="true">10</threshold>
        <append field="alert_type">suspicious_network</append>
        <append type="PLUGIN" field="geo_location">get_geo(_$dest_ip)</append>
    </rule>
</root>
```

### 系统进程白名单
```xml
<root type="WHITELIST" name="system_whitelist">
    <rule id="system_processes_001" name="系统进程白名单">
        <filter field="data_type">59</filter>
        <checklist condition="system_path and known_process">
            <node id="system_path" type="START" field="exe_path">C:\Windows\System32</node>
            <node id="known_process" type="INCL" field="exe" logic="OR" delimiter="|">svchost.exe|explorer.exe|winlogon.exe</node>
        </checklist>
        <append field="whitelist_category">system_processes</append>
    </rule>
</root>
```

### 异常登录检测
```xml
<root type="DETECTION" name="login_detection">
    <rule id="abnormal_login_001" name="异常登录检测">
        <filter field="event_type">login</filter>
        <checklist condition="failed_login and not_whitelisted">
            <node id="failed_login" type="EQU" field="status">failed</node>
            <node id="not_whitelisted" type="PLUGIN">not_in_whitelist(_$source_ip)</node>
        </checklist>
        <threshold group_by="username,source_ip" range="15m" local_cache="true">5</threshold>
        <append field="risk_level">medium</append>
        <append type="PLUGIN" field="user_info">get_user_profile(_$username)</append>
        <plugin>update_risk_score(_$username, "failed_login")</plugin>
    </rule>
</root>
```

---

## 🔧 常见问题排查

### XML语法错误
```xml
<!-- 错误：标签未闭合 -->
<rule id="test">
    <filter field="type">59</filter>

<!-- 正确：标签正确闭合 -->
<rule id="test">
    <filter field="type">59</filter>
</rule>
```

### XML特殊字符CDATA错误
```xml
<!-- 错误：正则包含XML特殊字符但未使用CDATA -->
<node type="REGEX" field="html"><script.*?>.*?</script></node>
<!-- 解析错误：< 和 > 被解析为XML标签 -->

<!-- 正确：使用CDATA包裹含特殊字符的正则 -->
<node type="REGEX" field="html"><![CDATA[<script.*?>.*?</script>]]></node>

<!-- 错误：filter值含特殊字符未用CDATA -->
<filter field="request_data"><form method="post"></filter>

<!-- 正确：filter值含特殊字符用CDATA -->
<filter field="request_data"><![CDATA[<form method="post">]]></filter>

<!-- 错误：append值含特殊字符未用CDATA -->
<append field="template"><div class="alert">Warning</div></append>

<!-- 正确：append值含特殊字符用CDATA -->
<append field="template"><![CDATA[<div class="alert">Warning</div>]]></append>

<!-- 错误：node值含特殊字符未用CDATA -->
<node type="INCL" field="data">value<test&data</node>

<!-- 正确：node值含特殊字符用CDATA -->
<node type="INCL" field="data"><![CDATA[value<test&data]]></node>

<!-- 正确：简单内容无需CDATA -->
<node type="REGEX" field="ip">^\d+\.\d+\.\d+\.\d+$</node>
<filter field="data_type">59</filter>
<append field="level">HIGH</append>
```

### 条件逻辑错误
```xml
<!-- 错误：使用未定义的节点ID -->
<checklist condition="a and b">
    <node id="x" type="INCL" field="exe">test</node>
</checklist>

<!-- 正确：条件中的ID必须存在 -->
<checklist condition="x">
    <node id="x" type="INCL" field="exe">test</node>
</checklist>
```

### 阈值配置错误
```xml
<!-- 错误：SUM类型缺少count_field -->
<threshold group_by="ip" range="5m" count_type="SUM">10</threshold>

<!-- 正确：SUM类型需要count_field -->
<threshold group_by="ip" range="5m" count_type="SUM" count_field="bytes">10</threshold>
```

### 必需属性缺失错误
```xml
<!-- 错误：rule缺少必需的id属性 -->
<rule name="test_rule">
    <filter field="type">59</filter>
</rule>

<!-- 正确：rule必须有id属性 -->
<rule id="test_rule_001" name="test_rule">
    <filter field="type">59</filter>
</rule>

<!-- 错误：node缺少必需的type属性 -->
<node field="exe">malware</node>

<!-- 正确：node必须有type属性 -->
<node type="INCL" field="exe">malware</node>

<!-- 错误：threshold缺少必需属性 -->
<threshold local_cache="true">10</threshold>

<!-- 正确：threshold必须有group_by和range -->
<threshold group_by="source_ip" range="5m" local_cache="true">10</threshold>

<!-- 错误：指定logic但缺少delimiter -->
<node type="INCL" field="process" logic="OR">malware.exe|virus.exe</node>

<!-- 正确：有logic时必须指定delimiter -->
<node type="INCL" field="process" logic="OR" delimiter="|">malware.exe|virus.exe</node>
```

### Filter可选性示例
```xml
<!-- 正确：有filter的规则 -->
<rule id="with_filter" name="带过滤器的规则">
    <filter field="data_type">59</filter>
    <checklist>
        <node type="INCL" field="exe">test</node>
    </checklist>
</rule>

<!-- 正确：无filter的规则（性能较低但有效） -->
<rule id="no_filter" name="无过滤器的规则">
    <checklist>
        <node type="INCL" field="exe">test</node>
    </checklist>
</rule>

<!-- 正确：空filter的规则（等同于无filter） -->
<rule id="empty_filter" name="空过滤器的规则">
    <filter field=""></filter>
    <checklist>
        <node type="INCL" field="exe">test</node>
    </checklist>
</rule>
```

---

## 📖 总结

通过本指南，您可以：
1. 理解AgentSmith-HUB规则引擎的核心概念
2. 掌握完整的XML规则语法
3. 学会创建高效的检测规则
4. 避免常见的配置错误（特别是CDATA的正确使用）
5. 实现复杂的业务检测逻辑

建议从简单规则开始，逐步掌握高级功能，并充分利用Web界面的实时验证功能进行测试和调试。

**⚠️ 重要提醒**: 编写规则时，请特别注意XML特殊字符的处理，任何包含`<>&"'`的内容都要用CDATA包裹，避免解析错误。
