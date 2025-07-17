# 🛡️ AgentSmith-HUB 规则引擎完整指南

AgentSmith-HUB 规则引擎是一个强大的实时数据处理引擎，它能够：
- 🔍 **实时检测**：从数据流中识别威胁和异常
- 🔄 **数据转换**：对数据进行加工和丰富化
- 📊 **统计分析**：进行阈值检测和频率分析
- 🚨 **自动响应**：触发告警和自动化操作

### 核心理念：灵活的执行顺序

规则引擎采用**灵活的执行顺序**，操作按照在XML中的出现顺序执行，让你可以根据具体需求自由组合各种操作。

## 📚 第一部分：从零开始

### 1.1 你的第一个规则

假设我们有这样的数据流入：
```json
{
  "event_type": "login",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "timestamp": 1699999999
}
```

最简单的规则：检测admin登录
```xml
<root author="beginner">
    <rule id="detect_admin_login" name="检测管理员登录">
        <!-- 独立的check，不需要checklist包装 -->
        <check type="EQU" field="username">admin</check>
        
        <!-- 添加标记 -->
        <append field="alert">admin login detected</append>
    </rule>
</root>
```

#### 🔍 语法详解：`<check>` 标签

`<check>` 是规则引擎中最基础的检查单元，用于对数据进行条件判断。

**基本语法：**
```xml
<check type="检查类型" field="字段名">比较值</check>
```

**属性说明：**
- `type`（必需）：指定检查类型，如 `EQU`（相等）、`INCL`（包含）、`REGEX`（正则匹配）等
- `field`（必需）：要检查的数据字段路径
- 标签内容：用于比较的值

**工作原理：**
1. 规则引擎从输入数据中提取 `field` 指定的字段值
2. 使用 `type` 指定的比较方式，将字段值与标签内容进行比较
3. 返回 true 或 false 的检查结果

#### 🔍 语法详解：`<append>` 标签

`<append>` 用于向数据中添加新字段或修改现有字段。

**基本语法：**
```xml
<append field="字段名">要添加的值</append>
```

**属性说明：**
- `field`（必需）：要添加或修改的字段名
- `type`（可选）：当值为 "PLUGIN" 时，表示使用插件生成值

**工作原理：**
当规则匹配成功后，`<append>` 操作会执行，向数据中添加指定的字段和值。

输出数据将变成：
```json
{
  "event_type": "login",
  "username": "admin", 
  "source_ip": "192.168.1.100",
  "timestamp": 1699999999,
  "alert": "admin login detected"  // 新添加的字段
}
```

### 1.2 添加更多检查条件

输入数据：
```json
{
  "event_type": "login",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "login_time": 23,  // 23点（晚上11点）
  "failed_attempts": 5
}
```

检测异常时间的admin登录：
```xml
<root author="learner">
    <rule id="suspicious_admin_login" name="可疑管理员登录">
        <!-- 灵活顺序：先检查用户名 -->
        <check type="EQU" field="username">admin</check>
        
        <!-- 再检查时间（深夜） -->
        <check type="MT" field="login_time">22</check>  <!-- 大于22点 -->
        
        <!-- 或者检查失败次数 -->
        <check type="MT" field="failed_attempts">3</check>
        
        <!-- 所有check默认是AND关系，全部满足才继续 -->
        
        <!-- 添加告警信息 -->
        <append field="risk_level">high</append>
        <append field="alert_reason">admin login at unusual time</append>
        
        <!-- 触发告警插件（假设已配置好） -->
        <plugin>send_security_alert(_$ORIDATA)</plugin>
    </rule>
</root>
```

#### 💡 重要概念：多条件检查的默认逻辑

当一个规则中有多个 `<check>` 标签时：
- 默认使用 **AND** 逻辑：所有检查都必须通过，规则才匹配
- 检查按顺序执行：如果某个检查失败，后续检查不会执行（短路求值）
- 这种设计提高了性能：尽早失败，避免不必要的检查

在上面的例子中，三个检查条件必须**全部满足**：
1. username 等于 "admin" 
2. login_time 大于 22（晚上10点后）
3. failed_attempts 大于 3

#### 🔍 语法详解：`<plugin>` 标签

`<plugin>` 用于执行自定义操作，通常用于响应动作。

**基本语法：**
```xml
<plugin>插件名称(参数1, 参数2, ...)</plugin>
```

**特点：**
- 执行操作但不返回值到数据中
- 通常用于外部动作：发送告警、执行阻断、记录日志等
- 只在规则匹配成功后执行

**与 `<append type="PLUGIN">` 的区别：**
- `<plugin>`：执行操作，不返回值
- `<append type="PLUGIN">`：执行插件并将返回值添加到数据中

### 1.3 使用动态值

输入数据：
```json
{
  "event_type": "transaction",
  "amount": 10000,
  "user": {
    "id": "user123",
    "daily_limit": 5000,
    "vip_level": "gold"
  }
}
```

检测超过用户限额的交易：
```xml
<root author="dynamic_learner">
    <rule id="over_limit_transaction" name="超限交易检测">
        <!-- 动态比较：交易金额 > 用户日限额 -->
        <check type="MT" field="amount">_$user.daily_limit</check>
        
        <!-- 使用插件计算超出比例（假设有自定义插件） -->
        <append type="PLUGIN" field="over_ratio">
            calculate_ratio(_$amount, _$user.daily_limit)
        </append>
        
        <!-- 根据VIP等级添加不同处理 -->
        <check type="EQU" field="user.vip_level">gold</check>
        <append field="action">notify_vip_service</append>
    </rule>
</root>
```

#### 🔍 语法详解：动态引用（_$ 前缀）

`_$` 前缀用于动态引用数据中的其他字段值，而不是使用静态的字符串。

**语法格式：**
- `_$字段名`：引用单个字段
- `_$父字段.子字段`：引用嵌套字段
- `_$ORIDATA`：引用整个原始数据对象

**工作原理：**
1. 当规则引擎遇到 `_$` 前缀时，会将其识别为动态引用
2. 从当前处理的数据中提取对应字段的值
3. 使用提取的值进行比较或处理

**在上面的例子中：**
- `_$user.daily_limit` 从数据中提取 `user.daily_limit` 的值（5000）
- `_$amount` 提取 `amount` 字段的值（10000）
- 动态比较：10000 > 5000，条件满足

**常见用法：**
```xml
<!-- 动态比较两个字段 -->
<check type="NEQ" field="current_user">_$login_user</check>

<!-- 在 append 中使用动态值 -->
<append field="message">User _$username logged in from _$source_ip</append>

<!-- 在插件参数中使用 -->
<plugin>blockIP(_$malicious_ip, _$block_duration)</plugin>
```

**_$ORIDATA 的使用：**
`_$ORIDATA` 代表整个原始数据对象，常用于：
- 将完整数据传递给插件进行复杂处理
- 生成包含所有信息的告警
- 数据备份或归档

```xml
<!-- 将整个数据对象发送给分析插件 -->
<append type="PLUGIN" field="risk_analysis">analyzeFullData(_$ORIDATA)</append>

<!-- 生成完整告警 -->
<plugin>sendAlert(_$ORIDATA, "HIGH_RISK")</plugin>
```

## 📊 第二部分：数据处理进阶

### 2.1 灵活的执行顺序

规则引擎的一大特点是灵活的执行顺序：

```xml
<rule id="flexible_way" name="灵活处理示例">
    <!-- 可以先添加时间戳 -->
    <append type="PLUGIN" field="check_time">now()</append>
    
    <!-- 然后进行检查 -->
    <check type="EQU" field="event_type">security_event</check>
    
    <!-- 统计阈值可以放在任何位置 -->
    <threshold group_by="source_ip" range="5m" value="10"/>
    
    <!-- 继续其他检查（假设有自定义插件） -->
    <check type="PLUGIN">is_working_hours(_$check_time)</check>
    
    <!-- 最后处理 -->
    <append field="processed">true</append>
</rule>
```

#### 💡 重要概念：执行顺序的意义

**为什么执行顺序很重要？**

1. **数据增强**：可以先添加字段，然后基于新字段做检查
2. **性能优化**：将快速检查放在前面，复杂操作放在后面
3. **条件处理**：某些操作可能依赖前面操作的结果

**执行流程：**
1. 规则引擎按照 XML 中标签的出现顺序执行操作
2. 检查类操作（check、threshold）如果失败，规则立即结束
3. 处理类操作（append、del、plugin）只在所有检查通过后执行

#### 🔍 语法详解：`<threshold>` 标签

`<threshold>` 用于检测在指定时间窗口内事件发生的频率。

**基本语法：**
```xml
<threshold group_by="分组字段" range="时间范围" value="阈值"/>
```

**属性说明：**
- `group_by`（必需）：按哪个字段分组统计，可以多个字段用逗号分隔
- `range`（必需）：时间窗口，支持 s(秒)、m(分)、h(时)、d(天)
- `value`（必需）：触发阈值，达到这个数量时检查通过

**工作原理：**
1. 按 `group_by` 字段对事件分组（如按 source_ip 分组）
2. 在 `range` 指定的滑动时间窗口内统计每组的事件数量
3. 当某组的统计值达到 `value` 时，该检查通过

**在上面的例子中：**
- 按 source_ip 分组
- 统计 5 分钟内的事件数
- 如果某个 IP 在 5 分钟内触发 10 次，则阈值检查通过

### 2.2 复杂的嵌套数据处理

输入数据：
```json
{
  "request": {
    "method": "POST",
    "url": "https://api.example.com/transfer",
    "headers": {
      "user-agent": "Mozilla/5.0...",
      "authorization": "Bearer token123"
    },
    "body": {
      "from_account": "ACC001",
      "to_account": "ACC999",
      "amount": 50000,
      "metadata": {
        "source": "mobile_app",
        "geo": {
          "country": "CN",
          "city": "Shanghai"
        }
      }
    }
  },
  "timestamp": 1700000000
}
```

处理嵌套数据的规则：
```xml
<root type="DETECTION" author="advanced">
    <rule id="complex_transaction_check" name="复杂交易检测">
        <!-- 检查基本条件 -->
        <check type="EQU" field="request.method">POST</check>
        <check type="INCL" field="request.url">transfer</check>
        
        <!-- 检查金额 -->
        <check type="MT" field="request.body.amount">10000</check>
        
        <!-- 添加地理位置标记 -->
        <append field="geo_risk">_$request.body.metadata.geo.country</append>
        
        <!-- 基于地理位置的阈值检测 -->
        <threshold group_by="request.body.from_account,request.body.metadata.geo.country" 
                   range="1h" value="3"/>
        
        <!-- 使用插件进行深度分析（假设有自定义插件） -->
        <check type="PLUGIN">analyze_transfer_risk(_$request.body)</check>
        
        <!-- 提取和处理user-agent -->
        <append type="PLUGIN" field="client_info">parseUA(_$request.headers.user-agent)</append>
        
        <!-- 清理敏感信息 -->
        <del>request.headers.authorization</del>
    </rule>
</root>
```

#### 🔍 语法详解：`<del>` 标签

`<del>` 用于从数据中删除指定的字段。

**基本语法：**
```xml
<del>字段1,字段2,字段3</del>
```

**特点：**
- 使用逗号分隔多个字段
- 支持嵌套字段路径：`user.password,session.token`
- 如果字段不存在，不会报错，静默忽略
- 只在规则匹配成功后执行

**使用场景：**
- 删除敏感信息（密码、token、密钥等）
- 清理临时字段
- 减少数据体积，避免传输不必要的信息

**在上面的例子中：**
- `request.headers.authorization` 包含敏感的认证信息
- 使用 `<del>` 在数据处理后删除该字段
- 确保敏感信息不会被存储或传输

### 2.3 条件组合逻辑

输入数据：
```json
{
  "event_type": "file_upload",
  "filename": "document.exe",
  "size": 1048576,
  "source": "email_attachment",
  "sender": "unknown@suspicious.com",
  "hash": "a1b2c3d4..."
}
```

使用条件组合的规则：
```xml
<root author="logic_master">
    <rule id="malware_detection" name="恶意软件检测">
        <!-- 方式1：使用独立check（默认AND关系） -->
        <check type="END" field="filename">.exe</check>
        <check type="MT" field="size">1000000</check>  <!-- 大于1MB -->
        
        <!-- 方式2：使用checklist进行复杂逻辑组合 -->
        <checklist condition="suspicious_file and (email_threat or unknown_hash)">
            <check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
                .exe|.dll|.scr|.bat
            </check>
            <check id="email_threat" type="INCL" field="sender">suspicious.com</check>
            <check id="unknown_hash" type="PLUGIN">
                is_known_malware(_$hash)
            </check>
</checklist>
        
        <!-- 丰富化数据 -->
        <append type="PLUGIN" field="virus_scan">virusTotal(_$hash)</append>
        <append field="threat_level">high</append>
        
        <!-- 自动响应（假设有自定义插件） -->
        <plugin>quarantine_file(_$filename)</plugin>
        <plugin>notify_security_team(_$ORIDATA)</plugin>
    </rule>
</root>
```

#### 🔍 语法详解：`<checklist>` 标签

`<checklist>` 允许你使用自定义的逻辑表达式组合多个检查条件。

**基本语法：**
```xml
<checklist condition="逻辑表达式">
    <check id="标识符1" ...>...</check>
    <check id="标识符2" ...>...</check>
</checklist>
```

**属性说明：**
- `condition`（必需）：使用检查节点的 `id` 构建的逻辑表达式

**逻辑表达式语法：**
- 使用 `and`、`or` 连接条件
- 使用 `()` 分组，控制优先级
- 使用 `not` 取反
- 只能使用小写的逻辑操作符

**示例表达式：**
- `a and b and c`：所有条件都满足
- `a or b or c`：任一条件满足
- `(a or b) and not c`：a或b满足，且c不满足
- `a and (b or (c and d))`：复杂嵌套条件

**工作原理：**
1. 执行所有带 `id` 的检查节点，记录每个节点的结果（true/false）
2. 将结果代入 `condition` 表达式计算最终结果
3. 如果最终结果为 true，则 checklist 通过

#### 🔍 语法详解：多值匹配（logic 和 delimiter）

当需要检查一个字段是否匹配多个值时，可以使用多值匹配语法。

**基本语法：**
```xml
<check type="类型" field="字段" logic="OR|AND" delimiter="分隔符">
    值1分隔符值2分隔符值3
</check>
```

**属性说明：**
- `logic`："OR" 或 "AND"，指定多个值之间的逻辑关系
- `delimiter`：分隔符，用于分割多个值

**工作原理：**
1. 使用 `delimiter` 将标签内容分割成多个值
2. 对每个值分别进行检查
3. 根据 `logic` 决定最终结果：
   - `logic="OR"`：任一值匹配即返回 true
   - `logic="AND"`：所有值都匹配才返回 true

**在上面的例子中：**
```xml
<check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
    .exe|.dll|.scr|.bat
</check>
```
- 检查 filename 是否包含 .exe、.dll、.scr 或 .bat
- 使用 OR 逻辑：任一扩展名匹配即可
- 使用 | 作为分隔符

## 🔧 第三部分：高级特性详解

### 3.1 阈值检测的三种模式

`<threshold>` 标签不仅可以简单计数，还支持三种强大的统计模式：

1. **默认模式（计数）**：统计事件发生次数
2. **SUM 模式**：对指定字段求和
3. **CLASSIFY 模式**：统计不同值的数量（去重计数）

#### 场景1：登录失败次数统计（默认计数）

输入数据流：
```json
// 10:00
{"event": "login_failed", "user": "john", "ip": "1.2.3.4"}
// 10:01
{"event": "login_failed", "user": "john", "ip": "1.2.3.4"}
// 10:02
{"event": "login_failed", "user": "john", "ip": "1.2.3.4"}
// 10:03
{"event": "login_failed", "user": "john", "ip": "1.2.3.4"}
// 10:04
{"event": "login_failed", "user": "john", "ip": "1.2.3.4"}
```

规则：
```xml
<rule id="brute_force_detection" name="暴力破解检测">
    <check type="EQU" field="event">login_failed</check>
    
    <!-- 5分钟内同一用户和IP失败5次 -->
    <threshold group_by="user,ip" range="5m" value="5"/>
    
    <append field="alert_type">brute_force_attempt</append>
    <plugin>block_ip(_$ip, 3600)</plugin>  <!-- 封禁1小时 -->
</rule>
```

#### 场景2：交易金额统计（SUM模式）

输入数据流：
```json
// 今天的交易
{"event": "transfer", "user": "alice", "amount": 5000}
{"event": "transfer", "user": "alice", "amount": 8000}
{"event": "transfer", "user": "alice", "amount": 40000}  // 累计53000，触发！
```

规则：
```xml
<rule id="daily_limit_check" name="日限额检测">
    <check type="EQU" field="event">transfer</check>
    
    <!-- 24小时内累计金额超过50000 -->
    <threshold group_by="user" range="24h" count_type="SUM" 
               count_field="amount" value="50000"/>
    
    <append field="action">freeze_account</append>
</rule>
```

#### 🔍 高级语法：threshold 的 SUM 模式

**属性说明：**
- `count_type="SUM"`：启用求和模式
- `count_field`（必需）：要求和的字段名
- `value`：当累计和达到此值时触发

**工作原理：**
1. 按 `group_by` 分组
2. 在时间窗口内累加 `count_field` 的值
3. 当累计值达到 `value` 时触发

#### 场景3：访问资源统计（CLASSIFY模式）

输入数据流：
```json
{"user": "bob", "action": "download", "file_id": "doc001"}
{"user": "bob", "action": "download", "file_id": "doc002"}
{"user": "bob", "action": "download", "file_id": "doc003"}
// ... 访问了26个不同文件
```

规则：
```xml
<rule id="data_exfiltration_check" name="数据外泄检测">
    <check type="EQU" field="action">download</check>
    
    <!-- 1小时内访问超过25个不同文件 -->
    <threshold group_by="user" range="1h" count_type="CLASSIFY" 
               count_field="file_id" value="25"/>
    
    <append field="risk_score">high</append>
    <plugin>alert_dlp_team(_$ORIDATA)</plugin>
</rule>
```

#### 🔍 高级语法：threshold 的 CLASSIFY 模式

**属性说明：**
- `count_type="CLASSIFY"`：启用去重计数模式
- `count_field`（必需）：要统计不同值的字段
- `value`：当不同值数量达到此值时触发

**工作原理：**
1. 按 `group_by` 分组
2. 在时间窗口内收集 `count_field` 的所有不同值
3. 当不同值的数量达到 `value` 时触发

**使用场景：**
- 检测扫描行为（访问多个不同端口/IP）
- 数据外泄检测（访问多个不同文件）
- 异常行为检测（使用多个不同账号）

### 3.2 内置插件系统

AgentSmith-HUB 提供了丰富的内置插件，无需额外开发即可使用。

#### 🧩 内置插件完整列表

##### 检查类插件（用于条件判断）
可在 `<check type="PLUGIN">` 中使用，返回布尔值。支持使用 `!` 前缀对结果取反，例如 `<check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>` 表示当IP不是私有地址时条件成立。

| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `isPrivateIP` | 检查IP是否为私有地址 | ip (string) | `<check type="PLUGIN">isPrivateIP(_$source_ip)</check>` |
| `cidrMatch` | 检查IP是否在CIDR范围内 | ip (string), cidr (string) | `<check type="PLUGIN">cidrMatch(_$client_ip, "192.168.1.0/24")</check>` |
| `geoMatch` | 检查IP是否属于指定国家 | ip (string), countryISO (string) | `<check type="PLUGIN">geoMatch(_$source_ip, "US")</check>` |
| `suppressOnce` | 告警抑制：时间窗口内只触发一次 | key (any), windowSec (int), ruleid (string, 可选) | `<check type="PLUGIN">suppressOnce(_$alert_key, 300, "rule_001")</check>` |

**注意插件参数格式**：
- 当引用数据中的字段时，使用 `_$` 前缀：`_$source_ip`
- 当使用静态值时，直接使用字符串（带引号）：`"192.168.1.0/24"`
- 当使用数字时，不需要引号：`300`

##### 数据处理插件（用于数据转换）
可在 `<append type="PLUGIN">` 中使用，返回各种类型的值：

**时间处理插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `now` | 获取当前时间戳 | 可选: format (unix/ms/rfc3339) | `<append type="PLUGIN" field="timestamp">now()</append>` |
| `ago` | 获取N秒前的时间戳 | seconds (int/float/string) | `<append type="PLUGIN" field="past_time">ago(3600)</append>` |
| `dayOfWeek` | 获取星期几(0-6, 0=周日) | 可选: timestamp (int64) | `<append type="PLUGIN" field="weekday">dayOfWeek()</append>` |
| `hourOfDay` | 获取小时(0-23) | 可选: timestamp (int64) | `<append type="PLUGIN" field="hour">hourOfDay()</append>` |
| `tsToDate` | 时间戳转RFC3339格式 | timestamp (int64) | `<append type="PLUGIN" field="formatted_time">tsToDate(_$event_time)</append>` |

**编码和哈希插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `base64Encode` | Base64编码 | input (string) | `<append type="PLUGIN" field="encoded">base64Encode(_$raw_data)</append>` |
| `base64Decode` | Base64解码 | encoded (string) | `<append type="PLUGIN" field="decoded">base64Decode(_$encoded_data)</append>` |
| `hashMD5` | 计算MD5哈希 | input (string) | `<append type="PLUGIN" field="md5">hashMD5(_$password)</append>` |
| `hashSHA1` | 计算SHA1哈希 | input (string) | `<append type="PLUGIN" field="sha1">hashSHA1(_$content)</append>` |
| `hashSHA256` | 计算SHA256哈希 | input (string) | `<append type="PLUGIN" field="sha256">hashSHA256(_$file_data)</append>` |

**URL处理插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `extractDomain` | 从URL提取域名 | urlOrHost (string) | `<append type="PLUGIN" field="domain">extractDomain(_$request_url)</append>` |
| `extractTLD` | 从域名提取顶级域名 | domain (string) | `<append type="PLUGIN" field="tld">extractTLD(_$hostname)</append>` |
| `extractSubdomain` | 从主机名提取子域名 | host (string) | `<append type="PLUGIN" field="subdomain">extractSubdomain(_$full_hostname)</append>` |

**字符串处理插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `replace` | 字符串替换 | input (string), old (string), new (string) | `<append type="PLUGIN" field="cleaned">replace(_$raw_text, "bad", "good")</append>` |
| `regexExtract` | 正则表达式提取 | input (string), pattern (string) | `<append type="PLUGIN" field="extracted">regexExtract(_$log_line, "IP: (\\d+\\.\\d+\\.\\d+\\.\\d+)")</append>` |
| `regexReplace` | 正则表达式替换 | input (string), pattern (string), replacement (string) | `<append type="PLUGIN" field="masked">regexReplace(_$email, "(.+)@(.+)", "$1@***")</append>` |

**数据解析插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `parseJSON` | 解析JSON字符串 | jsonString (string) | `<append type="PLUGIN" field="parsed">parseJSON(_$json_data)</append>` |
| `parseUA` | 解析User-Agent | userAgent (string) | `<append type="PLUGIN" field="browser_info">parseUA(_$user_agent)</append>` |

**威胁情报插件**
| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `virusTotal` | 查询VirusTotal文件哈希威胁情报 | hash (string), apiKey (string, 可选) | `<append type="PLUGIN" field="vt_scan">virusTotal(_$file_hash)</append>` |
| `shodan` | 查询Shodan IP地址基础设施情报 | ip (string), apiKey (string, 可选) | `<append type="PLUGIN" field="shodan_intel">shodan(_$ip_address)</append>` |
| `threatBook` | 查询微步在线威胁情报 | queryValue (string), queryType (string), apiKey (string, 可选) | `<append type="PLUGIN" field="tb_intel">threatBook(_$target_ip, "ip")</append>` |

**威胁情报插件配置说明**：
- API Key 可以在配置文件中统一设置，也可以在插件调用时传入
- 如果不提供 API Key，某些功能可能受限
- 建议在系统配置中统一管理 API Key，避免在规则中硬编码

#### 内置插件使用示例

##### 网络安全场景

输入数据：
```json
{
  "event_type": "network_connection",
  "source_ip": "10.0.0.100",
  "dest_ip": "185.220.101.45",
  "dest_port": 443,
  "bytes_sent": 1024000,
  "connection_duration": 3600
}
```

使用内置插件的规则：
```xml
<rule id="suspicious_connection" name="可疑连接检测">
        <!-- 检查是否为外部连接 -->
    <check type="PLUGIN">isPrivateIP(_$source_ip)</check>  <!-- 源是内网 -->
    <check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>  <!-- 目标是外网 -->
    
        <!-- 检查地理位置 -->
    <append type="PLUGIN" field="dest_country">geoMatch(_$dest_ip)</append>
    
    <!-- 添加时间戳 -->
    <append type="PLUGIN" field="detection_time">now()</append>
    <append type="PLUGIN" field="detection_hour">hourOfDay()</append>
    
    <!-- 计算数据外泄风险 -->
    <check type="MT" field="bytes_sent">1000000</check>  <!-- 大于1MB -->
    
    <!-- 生成告警 -->
    <append field="alert_type">potential_data_exfiltration</append>
    
    <!-- 查询威胁情报（如果有配置） -->
    <append type="PLUGIN" field="threat_intel">threatBook(_$dest_ip, "ip")</append>
</rule>
```

##### 威胁情报检测场景

展示灵活执行顺序的优势：先检查基础条件，再查询威胁情报，最后基于结果决策。

输入数据：
```json
{
  "event_type": "network_traffic",
  "datatype": "external_connection",
  "source_ip": "192.168.1.100",
  "dest_ip": "45.142.120.181",
  "dest_port": 443,
  "protocol": "tcp",
  "bytes_sent": 5000,
  "timestamp": 1700000000
}
```

威胁情报检测规则：
```xml
<rule id="threat_intel_detection" name="威胁情报检测">
    <!-- 第1步：检查数据类型，快速过滤 -->
    <check type="EQU" field="datatype">external_connection</check>
   
   <!-- 第2步：确认目标IP是公网地址 -->
   <check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>

   <!-- 第3步：查询威胁情报，增强数据 -->
    <append type="PLUGIN" field="threat_intel">threatBook(_$dest_ip, "ip")</append>
    
    <!-- 第4步：解析威胁情报结果 -->
    <append type="PLUGIN" field="threat_level">
        parseJSON(_$threat_intel).severity_level
    </append>
    
    <!-- 第5步：基于威胁等级进行判断 -->
    <checklist condition="high_threat or (medium_threat and has_data_transfer)">
        <check id="high_threat" type="EQU" field="threat_level">high</check>
        <check id="medium_threat" type="EQU" field="threat_level">medium</check>
        <check id="has_data_transfer" type="MT" field="bytes_sent">1000</check>
    </checklist>
    
    <!-- 第6步：丰富告警信息 -->
    <append field="alert_title">Malicious IP Communication Detected</append>
    <append type="PLUGIN" field="ip_reputation">
        parseJSON(_$threat_intel).reputation_score
    </append>
    <append type="PLUGIN" field="threat_tags">
        parseJSON(_$threat_intel).tags
    </append>
    
    <!-- 第7步：生成详细告警（假设有自定义插件） -->
    <plugin>generateThreatAlert(_$ORIDATA, _$threat_intel)</plugin>
</rule>
```

#### 💡 关键优势展示

这个示例展示了灵活执行顺序的几个关键优势：

1. **性能优化**：先执行快速检查（datatype），避免对所有数据查询威胁情报
2. **逐步增强**：先确认是公网IP，再查询威胁情报，避免无效查询
3. **动态决策**：基于威胁情报的返回结果动态调整后续处理
4. **条件响应**：只对高威胁等级执行响应操作
5. **数据利用**：充分利用威胁情报返回的丰富数据

如果使用固定执行顺序，就无法实现这种"先查询情报，再基于结果决策"的灵活处理方式。

##### 日志分析场景

输入数据：
```json
{
  "timestamp": 1700000000,
  "log_level": "ERROR",
  "message": "Failed login attempt",
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)...",
  "request_body": "{\"username\":\"admin\",\"password\":\"***\"}",
  "stack_trace": "java.lang.Exception: Authentication failed\n\tat com.example..."
}
```

日志处理规则：
```xml
<rule id="log_analysis" name="错误日志分析">
    <check type="EQU" field="log_level">ERROR</check>
    
    <!-- 解析JSON数据 -->
    <append type="PLUGIN" field="parsed_body">parseJSON(_$request_body)</append>
    
    <!-- 解析User-Agent -->
    <append type="PLUGIN" field="browser_info">parseUA(_$user_agent)</append>
    
    <!-- 提取错误信息 -->
    <append type="PLUGIN" field="error_type">
        regexExtract(_$stack_trace, "([A-Za-z.]+Exception)")
    </append>
    
    <!-- 时间处理 -->
    <append type="PLUGIN" field="readable_time">tsToDate(_$timestamp)</append>
    <append type="PLUGIN" field="hour">hourOfDay(_$timestamp)</append>
    
    <!-- 数据脱敏 -->
    <append type="PLUGIN" field="sanitized_message">
        regexReplace(_$message, "password\":\"[^\"]+", "password\":\"***")
    </append>
    
    <!-- 告警抑制：同类错误5分钟只报一次 -->
    <check type="PLUGIN">suppressOnce(_$error_type, 300, "error_log_analysis")</check>
    
    <!-- 生成告警（假设有自定义插件） -->
    <plugin>sendToElasticsearch(_$ORIDATA)</plugin>
</rule>
```

##### 数据脱敏和安全处理

```xml
<rule id="data_masking" name="数据脱敏处理">
    <check type="EQU" field="contains_sensitive_data">true</check>
    
    <!-- 数据哈希化 -->
    <append type="PLUGIN" field="user_id_hash">hashSHA256(_$user_id)</append>
    <append type="PLUGIN" field="session_hash">hashMD5(_$session_id)</append>
    
    <!-- 敏感信息编码 -->
    <append type="PLUGIN" field="encoded_payload">base64Encode(_$sensitive_payload)</append>
    
    <!-- 清理和替换 -->
    <append type="PLUGIN" field="cleaned_log">replace(_$raw_log, _$user_password, "***")</append>
    <append type="PLUGIN" field="masked_phone">regexReplace(_$phone_number, "(\\d{3})\\d{4}(\\d{4})", "$1****$2")</append>
    
    <!-- 删除原始敏感数据 -->
    <del>user_password,raw_sensitive_data,unencrypted_payload</del>
</rule>
```

#### ⚠️ 告警抑制最佳实践（suppressOnce）

告警抑制插件可以防止同一告警在短时间内重复触发。

**为什么需要 ruleid 参数？**

如果不使用 `ruleid` 参数，不同规则对同一key的抑制会相互影响：

```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(_$source_ip, 300)</check>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(_$source_ip, 300)</check>
</rule>
```

**问题**：规则A触发后，规则B对同一IP也会被抑制！

**正确用法**：使用 `ruleid` 参数隔离不同规则：

```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(_$source_ip, 300, "network_threat")</check>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(_$source_ip, 300, "login_anomaly")</check>
</rule>
```

#### 插件性能说明

性能等级（从高到低）：
1. **检查节点插件**：`isPrivateIP`, `cidrMatch` - 纯计算，性能较高
2. **字符串处理插件**：`replace`, `hashMD5/SHA1/SHA256` - 中等性能
3. **正则表达式插件**：`regexExtract`, `regexReplace` - 性能较低
4. **威胁情报插件**：`virusTotal`, `shodan`, `threatBook` - 外部API调用，性能最低

优化建议：
```xml
<!-- 推荐：先用高性能检查，再用低性能插件 -->
<rule id="optimized">
    <check type="INCL" field="alert_level">high</check>
    <check type="NOTNULL" field="source_ip"></check>
    <append type="PLUGIN" field="threat_intel">threatBook(_$source_ip, "ip")</append>
</rule>
```

### 3.3 白名单规则集

白名单用于过滤掉不需要处理的数据。白名单的特殊行为：
- 当白名单规则匹配时，数据被"允许通过"（即被过滤掉，不再继续处理）
- 当白名单规则不匹配时，数据继续传递给后续处理
- 白名单中的 `append`、`del`、`plugin` 操作不会执行（因为匹配的数据会被过滤）

```xml
<root type="WHITELIST" name="security_whitelist" author="security_team">
    <!-- 白名单规则1：信任的IP -->
    <rule id="trusted_ips">
        <check type="INCL" field="source_ip" logic="OR" delimiter="|">
            10.0.0.1|10.0.0.2|10.0.0.3
        </check>
        <!-- 注意：以下操作不会执行，因为匹配的数据会被过滤 -->
        <append field="whitelisted">true</append>
    </rule>
    
    <!-- 白名单规则2：已知的良性进程 -->
    <rule id="benign_processes">
        <check type="INCL" field="process_name" logic="OR" delimiter="|">
            chrome.exe|firefox.exe|explorer.exe
        </check>
        <!-- 可以添加多个检查条件，全部满足才会被白名单过滤 -->
        <check type="PLUGIN">isPrivateIP(_$source_ip)</check>
</rule>
    
    <!-- 白名单规则3：内部测试流量 -->
    <rule id="test_traffic">
        <check type="INCL" field="user_agent">internal-testing-bot</check>
        <check type="PLUGIN">cidrMatch(_$source_ip, "192.168.100.0/24")</check>
    </rule>
</root>
```

## 🚨 第四部分：实战案例集

### 案例1：APT攻击检测

完整的APT攻击检测规则集（使用内置插件和假设的自定义插件）：

```xml
<root type="DETECTION" name="apt_detection_suite" author="threat_hunting_team">
    <!-- 规则1：PowerShell Empire检测 -->
    <rule id="powershell_empire" name="PowerShell Empire C2检测">
        <!-- 灵活顺序：先enrichment再检测 -->
        <append type="PLUGIN" field="decoded_cmd">base64Decode(_$command_line)</append>
        
        <!-- 检查进程名 -->
        <check type="INCL" field="process_name">powershell</check>
        
        <!-- 检测Empire特征 -->
        <check type="INCL" field="decoded_cmd" logic="OR" delimiter="|">
            System.Net.WebClient|DownloadString|IEX|Invoke-Expression
        </check>
        
        <!-- 检测编码命令 -->
        <check type="INCL" field="command_line">-EncodedCommand</check>
        
        <!-- 网络连接检测 -->
        <threshold group_by="hostname" range="10m" value="3"/>
        
        <!-- 威胁情报查询 -->
        <append type="PLUGIN" field="c2_url">
            regexExtract(_$decoded_cmd, "https?://[^\\s]+")
        </append>
        
        <!-- 生成IoC -->
        <append field="ioc_type">powershell_empire_c2</append>
        <append type="PLUGIN" field="ioc_hash">hashSHA256(_$decoded_cmd)</append>
        
        <!-- 自动响应（假设有自定义插件） -->
        <plugin>isolateHost(_$hostname)</plugin>
        <plugin>extractAndShareIoCs(_$ORIDATA)</plugin>
    </rule>
    
    <!-- 规则2：横向移动检测 -->
    <rule id="lateral_movement" name="横向移动检测">
        <!-- 多种横向移动技术检测 -->
        <checklist condition="(wmi_exec or psexec or rdp_brute) and not internal_scan">
            <!-- WMI执行 -->
            <check id="wmi_exec" type="INCL" field="process_name">wmic.exe</check>
            <!-- PsExec -->
            <check id="psexec" type="INCL" field="service_name">PSEXESVC</check>
            <!-- RDP暴力破解 -->
            <check id="rdp_brute" type="EQU" field="event_id">4625</check>
            <!-- 排除内部扫描 -->
            <check id="internal_scan" type="PLUGIN">
                isPrivateIP(_$source_ip)
            </check>
</checklist>
        
        <!-- 时间窗口检测 -->
        <threshold group_by="source_ip,dest_ip" range="30m" value="5"/>
        
        <!-- 风险评分（假设有自定义插件） -->
        <append type="PLUGIN" field="risk_score">
            calculateLateralMovementRisk(_$ORIDATA)
        </append>
        
        <plugin>updateAttackGraph(_$source_ip, _$dest_ip)</plugin>
    </rule>
    
    <!-- 规则3：数据外泄检测 -->
    <rule id="data_exfiltration" name="数据外泄检测">
        <!-- 先检查是否为敏感数据访问 -->
        <check type="INCL" field="file_path" logic="OR" delimiter="|">
            /etc/passwd|/etc/shadow|.ssh/|.aws/credentials
        </check>

       <!-- 检查外联行为 -->
       <check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>
       
        <!-- 异常传输检测 -->
        <threshold group_by="source_ip" range="1h" count_type="SUM" 
                   count_field="bytes_sent" value="1073741824"/>  <!-- 1GB -->
        
        <!-- DNS隧道检测（假设有自定义插件） -->
        <checklist condition="dns_tunnel_check">
            <check id="dns_tunnel_check" type="PLUGIN">
                detectDNSTunnel(_$dns_queries)
            </check>
        </checklist>
        
        <!-- 生成告警 -->
        <append field="alert_severity">critical</append>
        <append type="PLUGIN" field="data_classification">
            classifyData(_$file_path)
        </append>
        
        <plugin>blockDataTransfer(_$source_ip, _$dest_ip)</plugin>
        <plugin>triggerIncidentResponse(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 案例2：金融欺诈实时检测

```xml
<root type="DETECTION" name="fraud_detection_system" author="risk_team">
    <!-- 规则1：账户接管检测 -->
    <rule id="account_takeover" name="账户接管检测">
        <!-- 实时设备指纹（假设有自定义插件） -->
        <append type="PLUGIN" field="device_fingerprint">
            generateFingerprint(_$user_agent, _$screen_resolution, _$timezone)
        </append>
        
        <!-- 检查设备变更（假设有自定义插件） -->
        <check type="PLUGIN">
            isNewDevice(_$user_id, _$device_fingerprint)
        </check>
        
        <!-- 地理位置异常（假设有自定义插件） -->
        <append type="PLUGIN" field="geo_distance">
            calculateGeoDistance(_$user_id, _$current_ip, _$last_ip)
        </append>
        <check type="MT" field="geo_distance">500</check>  <!-- 500km -->
        
        <!-- 行为模式分析（假设有自定义插件） -->
        <append type="PLUGIN" field="behavior_score">
            analyzeBehaviorPattern(_$user_id, _$recent_actions)
        </append>
        
        <!-- 交易速度检测 -->
        <threshold group_by="user_id" range="10m" value="5"/>
        
        <!-- 风险决策（假设有自定义插件） -->
        <append type="PLUGIN" field="risk_decision">
            makeRiskDecision(_$behavior_score, _$geo_distance, _$device_fingerprint)
        </append>
        
        <!-- 实时干预（假设有自定义插件） -->
        <plugin>requireMFA(_$user_id, _$transaction_id)</plugin>
        <plugin>notifyUser(_$user_id, "suspicious_activity")</plugin>
    </rule>
    
    <!-- 规则2：洗钱行为检测 -->
    <rule id="money_laundering" name="洗钱行为检测">
        <!-- 分散-聚合模式（假设有自定义插件） -->
        <checklist condition="structuring or layering or integration">
            <!-- 结构化拆分 -->
            <check id="structuring" type="PLUGIN">
                detectStructuring(_$user_id, _$transaction_history)
            </check>
            <!-- 分层处理 -->
            <check id="layering" type="PLUGIN">
                detectLayering(_$account_network, _$transaction_flow)
            </check>
            <!-- 整合阶段 -->
            <check id="integration" type="PLUGIN">
                detectIntegration(_$merchant_category, _$transaction_pattern)
            </check>
        </checklist>
        
        <!-- 关联分析（假设有自定义插件） -->
        <append type="PLUGIN" field="network_risk">
            analyzeAccountNetwork(_$user_id, _$connected_accounts)
        </append>
        
        <!-- 累计金额监控 -->
        <threshold group_by="account_cluster" range="7d" count_type="SUM"
                   count_field="amount" value="1000000"/>
        
        <!-- 合规报告（假设有自定义插件） -->
        <append type="PLUGIN" field="sar_report">
            generateSAR(_$ORIDATA)  <!-- Suspicious Activity Report -->
        </append>
        
        <plugin>freezeAccountCluster(_$account_cluster)</plugin>
        <plugin>notifyCompliance(_$sar_report)</plugin>
    </rule>
</root>
```

### 案例3：零信任安全架构

```xml
<root type="DETECTION" name="zero_trust_security" author="security_architect">
    <!-- 规则1：持续身份验证 -->
    <rule id="continuous_auth" name="持续身份验证">
        <!-- 每次请求都验证 -->
        <check type="NOTNULL" field="auth_token"></check>
        
        <!-- 验证token有效性（假设有自定义插件） -->
        <check type="PLUGIN">validateToken(_$auth_token)</check>
        
        <!-- 上下文感知（假设有自定义插件） -->
        <append type="PLUGIN" field="trust_score">
            calculateTrustScore(
                _$user_id,
                _$device_trust,
                _$network_location,
                _$behavior_baseline,
                _$time_of_access
            )
        </append>
        
        <!-- 动态权限调整 -->
        <checklist condition="low_trust or anomaly_detected">
            <check id="low_trust" type="LT" field="trust_score">0.7</check>
            <check id="anomaly_detected" type="PLUGIN">
                detectAnomaly(_$current_behavior, _$baseline_behavior)
            </check>
    </checklist>
        
        <!-- 微分段策略（假设有自定义插件） -->
        <append type="PLUGIN" field="allowed_resources">
            applyMicroSegmentation(_$trust_score, _$requested_resource)
        </append>
        
        <!-- 实时策略执行（假设有自定义插件） -->
        <plugin>enforcePolicy(_$user_id, _$allowed_resources)</plugin>
        <plugin>logZeroTrustDecision(_$ORIDATA)</plugin>
</rule>
    
    <!-- 规则2：设备信任评估 -->
    <rule id="device_trust" name="设备信任评估">
        <!-- 设备健康检查（假设有自定义插件） -->
        <append type="PLUGIN" field="device_health">
            checkDeviceHealth(_$device_id)
        </append>
        
        <!-- 合规性验证（假设有自定义插件） -->
        <checklist condition="patch_level and antivirus and encryption and mdm_enrolled">
            <check id="patch_level" type="PLUGIN">
                isPatchCurrent(_$os_version, _$patch_level)
            </check>
            <check id="antivirus" type="PLUGIN">
                isAntivirusActive(_$av_status)
            </check>
            <check id="encryption" type="PLUGIN">
                isDiskEncrypted(_$device_id)
            </check>
            <check id="mdm_enrolled" type="PLUGIN">
                isMDMEnrolled(_$device_id)
            </check>
    </checklist>
        
        <!-- 证书验证（假设有自定义插件） -->
        <check type="PLUGIN">
            validateDeviceCertificate(_$device_cert)
        </check>
        
        <!-- 信任评分（假设有自定义插件） -->
        <append type="PLUGIN" field="device_trust_score">
            calculateDeviceTrust(_$ORIDATA)
        </append>
        
        <!-- 访问决策（假设有自定义插件） -->
        <plugin>applyDevicePolicy(_$device_id, _$device_trust_score)</plugin>
</rule>
</root>
```

## 📖 第五部分：语法参考手册

### 5.1 规则集结构

#### 根元素 `<root>`
```xml
<root type="DETECTION|WHITELIST" name="规则集名称" author="作者">
    <!-- 规则列表 -->
</root>
```

| 属性 | 必需 | 说明 | 默认值 |
|------|------|------|--------|
| type | 否 | 规则集类型 | DETECTION |
| name | 否 | 规则集名称 | - |
| author | 否 | 作者信息 | - |

#### 规则元素 `<rule>`
```xml
<rule id="唯一标识符" name="规则描述">
    <!-- 操作列表：按出现顺序执行 -->
</rule>
```

| 属性 | 必需 | 说明 |
|------|------|------|
| id | 是 | 规则唯一标识符 |
| name | 否 | 规则可读描述 |

### 5.2 检查操作

#### 独立检查 `<check>`
```xml
<check type="类型" field="字段名" logic="OR|AND" delimiter="分隔符">
    值
</check>
```

| 属性 | 必需 | 说明 | 适用场景 |
|------|------|------|----------|
| type | 是 | 检查类型 | 所有 |
| field | 条件 | 字段名（PLUGIN类型可选） | 非PLUGIN类型必需 |
| logic | 否 | 多值逻辑 | 使用分隔符时 |
| delimiter | 条件 | 值分隔符 | 使用logic时必需 |
| id | 条件 | 节点标识符 | 在checklist中使用condition时必需 |

#### 检查列表 `<checklist>`
```xml
<checklist condition="逻辑表达式">
    <check id="a" ...>...</check>
    <check id="b" ...>...</check>
</checklist>
```

| 属性 | 必需 | 说明 |
|------|------|------|
| condition | 否 | 逻辑表达式（如：`a and (b or c)`） |

### 5.3 检查类型完整列表

#### 字符串匹配类
| 类型 | 说明 | 大小写 | 示例 |
|------|------|--------|------|
| EQU | 完全相等 | 不敏感 | `<check type="EQU" field="status">active</check>` |
| NEQ | 完全不等 | 不敏感 | `<check type="NEQ" field="status">inactive</check>` |
| INCL | 包含子串 | 敏感 | `<check type="INCL" field="message">error</check>` |
| NI | 不包含子串 | 敏感 | `<check type="NI" field="message">success</check>` |
| START | 开头匹配 | 敏感 | `<check type="START" field="path">/admin</check>` |
| END | 结尾匹配 | 敏感 | `<check type="END" field="file">.exe</check>` |
| NSTART | 开头不匹配 | 敏感 | `<check type="NSTART" field="path">/public</check>` |
| NEND | 结尾不匹配 | 敏感 | `<check type="NEND" field="file">.txt</check>` |

#### 大小写忽略类
| 类型 | 说明 | 示例 |
|------|------|------|
| NCS_EQU | 忽略大小写相等 | `<check type="NCS_EQU" field="protocol">HTTP</check>` |
| NCS_NEQ | 忽略大小写不等 | `<check type="NCS_NEQ" field="method">get</check>` |
| NCS_INCL | 忽略大小写包含 | `<check type="NCS_INCL" field="header">Content-Type</check>` |
| NCS_NI | 忽略大小写不包含 | `<check type="NCS_NI" field="useragent">bot</check>` |
| NCS_START | 忽略大小写开头 | `<check type="NCS_START" field="domain">WWW.</check>` |
| NCS_END | 忽略大小写结尾 | `<check type="NCS_END" field="email">.COM</check>` |
| NCS_NSTART | 忽略大小写开头不匹配 | `<check type="NCS_NSTART" field="url">HTTP://</check>` |
| NCS_NEND | 忽略大小写结尾不匹配 | `<check type="NCS_NEND" field="filename">.EXE</check>` |

#### 数值比较类
| 类型 | 说明 | 示例 |
|------|------|------|
| MT | 大于 | `<check type="MT" field="score">80</check>` |
| LT | 小于 | `<check type="LT" field="age">18</check>` |

#### 空值检查类
| 类型 | 说明 | 示例 |
|------|------|------|
| ISNULL | 字段为空 | `<check type="ISNULL" field="optional_field"></check>` |
| NOTNULL | 字段非空 | `<check type="NOTNULL" field="required_field"></check>` |

#### 高级匹配类
| 类型 | 说明 | 示例 |
|------|------|------|
| REGEX | 正则表达式 | `<check type="REGEX" field="ip">^\d+\.\d+\.\d+\.\d+$</check>` |
| PLUGIN | 插件函数（支持 `!` 取反） | `<check type="PLUGIN">isValidEmail(_$email)</check>` |

### 5.4 数据处理操作

#### 阈值检测 `<threshold>`
```xml
<threshold group_by="字段1,字段2" range="时间范围" value="阈值" 
           count_type="SUM|CLASSIFY" count_field="统计字段" local_cache="true|false"/>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| group_by | 是 | 分组字段 | `source_ip,user_id` |
| range | 是 | 时间范围 | `5m`, `1h`, `24h` |
| value | 是 | 阈值 | `10` |
| count_type | 否 | 计数类型 | 默认：计数，`SUM`：求和，`CLASSIFY`：去重计数 |
| count_field | 条件 | 统计字段 | 使用SUM/CLASSIFY时必需 |
| local_cache | 否 | 使用本地缓存 | `true` 或 `false` |

#### 字段追加 `<append>`
```xml
<append field="字段名" type="PLUGIN">值或插件调用</append>
```

| 属性 | 必需 | 说明 |
|------|------|------|
| field | 是 | 要添加的字段名 |
| type | 否 | 追加类型（`PLUGIN`表示插件调用） |

#### 字段删除 `<del>`
```xml
<del>字段1,字段2,字段3</del>
```

#### 插件执行 `<plugin>`
```xml
<plugin>插件函数(参数1, 参数2)</plugin>
```

### 5.5 字段访问语法

#### 基本访问
- **直接字段**：`field_name`
- **嵌套字段**：`parent.child.grandchild`
- **数组索引**：`array.0.field`（访问第一个元素）

#### 动态引用（_$前缀）
- **字段引用**：`_$field_name`
- **嵌套引用**：`_$parent.child.field`
- **原始数据**：`_$ORIDATA`

#### 示例对比
```xml
<!-- 静态值 -->
<check type="EQU" field="status">active</check>

<!-- 动态值 -->
<check type="EQU" field="status">_$expected_status</check>

<!-- 嵌套字段 -->
<check type="EQU" field="user.profile.role">admin</check>

<!-- 动态嵌套 -->
<check type="EQU" field="current_level">_$config.min_level</check>
```

### 5.6 内置插件快速参考

#### 检查类插件（返回bool）
| 插件 | 功能 | 参数 | 示例 |
|------|------|------|------|
| isPrivateIP | 检查私有IP | ip | `isPrivateIP(_$ip)` |
| cidrMatch | CIDR匹配 | ip, cidr | `cidrMatch(_$ip, "10.0.0.0/8")` |
| geoMatch | 地理位置匹配 | ip, country | `geoMatch(_$ip, "US")` |
| suppressOnce | 告警抑制 | key, seconds, ruleid | `suppressOnce(_$ip, 300, "rule1")` |

#### 数据处理插件（返回各种类型）
| 插件 | 功能 | 返回类型 | 示例 |
|------|------|----------|------|
| now | 当前时间 | int64 | `now()` |
| base64Encode | Base64编码 | string | `base64Encode(_$data)` |
| hashSHA256 | SHA256哈希 | string | `hashSHA256(_$content)` |
| parseJSON | JSON解析 | object | `parseJSON(_$json_str)` |
| regexExtract | 正则提取 | string | `regexExtract(_$text, pattern)` |

### 5.7 性能优化建议

#### 操作顺序优化
```xml
<!-- 推荐：高性能操作在前 -->
<rule id="optimized">
    <check type="NOTNULL" field="required"></check>     <!-- 最快 -->
    <check type="EQU" field="type">target</check>       <!-- 快 -->
    <check type="INCL" field="message">keyword</check>  <!-- 中等 -->
    <check type="REGEX" field="data">pattern</check>    <!-- 慢 -->
    <check type="PLUGIN">complex_check()</check>        <!-- 最慢 -->
</rule>
```

#### 阈值配置优化
```xml
<!-- 使用本地缓存提升性能 -->
<threshold group_by="user_id" range="5m" value="10" local_cache="true"/>

<!-- 避免过大的时间窗口 -->
<threshold group_by="ip" range="1h" value="1000"/>  <!-- 不要超过24h -->
```

### 5.8 常见错误和解决方案

#### XML语法错误
```xml
<!-- 错误：特殊字符未转义 -->
<check type="INCL" field="xml"><tag>value</tag></check>

<!-- 正确：使用CDATA -->
<check type="INCL" field="xml"><![CDATA[<tag>value</tag>]]></check>
```

#### 逻辑错误
```xml
<!-- 错误：condition中引用不存在的id -->
<checklist condition="a and b">
    <check type="EQU" field="status">active</check>  <!-- 缺少id -->
</checklist>

<!-- 正确 -->
<checklist condition="a and b">
    <check id="a" type="EQU" field="status">active</check>
    <check id="b" type="NOTNULL" field="user"></check>
        </checklist>
```

#### 性能问题
```xml
<!-- 问题：在大量数据上直接使用插件 -->
<rule id="slow">
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>

<!-- 优化：先过滤后处理 -->
<rule id="fast">
    <check type="EQU" field="type">target</check>
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
    </rule>
```

### 5.9 调试技巧

#### 1. 使用append跟踪执行流程
```xml
<rule id="debug_flow">
    <append field="_debug_step1">check started</append>
    <check type="EQU" field="type">target</check>
    
    <append field="_debug_step2">check passed</append>
    <threshold group_by="user" range="5m" value="10"/>
    
    <append field="_debug_step3">threshold passed</append>
    <!-- 最终数据会包含所有debug字段，显示执行流程 -->
</rule>
```

#### 2. 测试单个规则
创建只包含待测试规则的规则集：
```xml
<root type="DETECTION" name="test_single_rule">
    <rule id="test_rule">
        <!-- 你的测试规则 -->
    </rule>
</root>
```

#### 3. 验证字段访问
使用append验证字段是否正确获取：
```xml
<rule id="verify_fields">
    <append field="debug_nested">_$user.profile.settings.theme</append>
    <append field="debug_array">_$items.0.name</append>
    <!-- 检查输出中的debug字段值 -->
</rule>
```

## 🔧 第六部分：自定义插件开发

### 6.1 插件分类

AgentSmith-HUB 支持两种类型的插件：

#### 插件运行方式分类
1. **本地插件（Local Plugin）**：编译到程序中的内置插件，性能最高
2. **Yaegi插件（Yaegi Plugin）**：使用 Yaegi 解释器运行的动态插件，灵活度最高

#### 插件返回类型分类
1. **检查类插件（Check Node Plugin）**：返回 `(bool, error)`，用于 `<check type="PLUGIN">` 中
2. **数据处理插件（Other Plugin）**：返回 `(interface{}, bool, error)`，用于 `<append type="PLUGIN">` 和 `<plugin>` 中

### 6.2 插件函数签名

#### 重要：Eval函数签名说明

插件必须定义一个名为 `Eval` 的函数，根据插件用途选择正确的函数签名：

**检查类插件签名**：
```go
func Eval(参数...) (bool, error)
```
- 第一个返回值：检查结果（true/false）
- 第二个返回值：错误信息（如果有）

**数据处理插件签名**：
```go
func Eval(参数...) (interface{}, bool, error)
```
- 第一个返回值：处理结果（任意类型）
- 第二个返回值：是否成功（true/false）
- 第三个返回值：错误信息（如果有）

### 6.3 编写自定义插件

#### 基本结构

```go
package plugin

import (
    "strings"
    "fmt"
)

// Eval 是插件的入口函数，必须定义此函数
// 根据插件用途选择合适的函数签名
```

#### 检查类插件示例

用于条件判断，返回 bool 值：

```go
package plugin

import (
    "strings"
    "fmt"
)

// 检查邮箱是否来自指定域名
// 返回 (bool, error) - 用于 check 节点
func Eval(email string, allowedDomain string) (bool, error) {
    if email == "" {
        return false, nil
    }
    
    // 提取邮箱域名
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return false, fmt.Errorf("invalid email format: %s", email)
    }
    
    domain := strings.ToLower(parts[1])
    allowed := strings.ToLower(allowedDomain)
    
    return domain == allowed, nil
}
```

使用示例：
```xml
<check type="PLUGIN">checkEmailDomain(_$email, "company.com")</check>
```

#### 数据处理插件示例

用于数据转换、计算等，返回任意类型：

```go
package plugin

import (
    "strings"
)

// 解析并提取User-Agent中的信息
// 返回 (interface{}, bool, error) - 用于 append 或 plugin 节点
func Eval(userAgent string) (interface{}, bool, error) {
    if userAgent == "" {
        return nil, false, nil
    }
    
    result := make(map[string]interface{})
    
    // 简单的浏览器检测
    if strings.Contains(userAgent, "Chrome") {
        result["browser"] = "Chrome"
    } else if strings.Contains(userAgent, "Firefox") {
        result["browser"] = "Firefox"
    } else if strings.Contains(userAgent, "Safari") {
        result["browser"] = "Safari"
    } else {
        result["browser"] = "Unknown"
    }
    
    // 操作系统检测
    if strings.Contains(userAgent, "Windows") {
        result["os"] = "Windows"
    } else if strings.Contains(userAgent, "Mac") {
        result["os"] = "macOS"
    } else if strings.Contains(userAgent, "Linux") {
        result["os"] = "Linux"
    } else {
        result["os"] = "Unknown"
    }
    
    // 是否移动设备
    result["is_mobile"] = strings.Contains(userAgent, "Mobile")
    
    return result, true, nil
}
```

使用示例：
```xml
<!-- 提取信息到新字段 -->
<append type="PLUGIN" field="ua_info">parseCustomUA(_$user_agent)</append>

<!-- 后续可以访问解析结果 -->
<check type="EQU" field="ua_info.browser">Chrome</check>
<check type="EQU" field="ua_info.is_mobile">true</check>
```

### 6.4 插件开发规范

#### 命名规范
- 插件名使用驼峰命名法：`isValidEmail`、`extractDomain`
- 检查类插件通常以 `is`、`has`、`check` 开头
- 处理类插件通常以动词开头：`parse`、`extract`、`calculate`

#### 参数设计
```go
// 推荐：参数明确，易于理解
func Eval(ip string, cidr string) (bool, error)

// 避免：参数过多
func Eval(a, b, c, d, e string) (bool, error)

// 支持可变参数
func Eval(ip string, cidrs ...string) (bool, error)
```

#### 错误处理
```go
func Eval(data string) (interface{}, bool, error) {
    // 输入验证
    if data == "" {
        return nil, false, nil  // 空输入返回 false，不报错
    }
    
    // 处理可能的错误
    result, err := processData(data)
    if err != nil {
        return nil, false, fmt.Errorf("process data failed: %w", err)
    }
    
    return result, true, nil
}
```

#### 性能考虑
```go
package plugin

import (
    "regexp"
    "sync"
)

// 使用全局变量缓存正则表达式
var (
    emailRegex *regexp.Regexp
    regexOnce  sync.Once
)

func Eval(email string) (bool, error) {
    // 确保正则只编译一次
    regexOnce.Do(func() {
        emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
    })
    
    return emailRegex.MatchString(email), nil
}
```

### 6.5 高级插件示例

#### 复杂数据处理插件

```go
package plugin

import (
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"
)

// 生成用户行为指纹
func Eval(userID string, actions string, timestamp int64) (interface{}, bool, error) {
    // 解析用户行为
    var actionList []map[string]interface{}
    if err := json.Unmarshal([]byte(actions), &actionList); err != nil {
        return nil, false, fmt.Errorf("invalid actions format: %w", err)
    }
    
    // 分析行为模式
    result := map[string]interface{}{
        "user_id": userID,
        "timestamp": timestamp,
        "action_count": len(actionList),
        "time_of_day": time.Unix(timestamp, 0).Hour(),
    }
    
    // 计算行为频率
    actionTypes := make(map[string]int)
    for _, action := range actionList {
        if actionType, ok := action["type"].(string); ok {
            actionTypes[actionType]++
        }
    }
    result["action_types"] = actionTypes
    
    // 生成行为指纹
    fingerprint := fmt.Sprintf("%s-%d-%v", userID, len(actionList), actionTypes)
    hash := md5.Sum([]byte(fingerprint))
    result["fingerprint"] = hex.EncodeToString(hash[:])
    
    // 风险评分
    riskScore := 0
    if len(actionList) > 100 {
        riskScore += 20
    }
    if hour := result["time_of_day"].(int); hour < 6 || hour > 22 {
        riskScore += 30
    }
    result["risk_score"] = riskScore
    
    return result, true, nil
}
```

#### 状态管理插件

```go
package plugin

import (
    "sync"
    "time"
)

var (
    requestCount = make(map[string]*userRequest)
    mu          sync.RWMutex
)

type userRequest struct {
    count      int
    lastUpdate time.Time
}

// 检测用户请求频率是否异常
func Eval(userID string, threshold int) (bool, error) {
    mu.Lock()
    defer mu.Unlock()
    
    now := time.Now()
    
    // 获取或创建用户记录
    req, exists := requestCount[userID]
    if !exists {
        req = &userRequest{
            count:      1,
            lastUpdate: now,
        }
        requestCount[userID] = req
        return false, nil
    }
    
    // 如果距离上次请求超过1分钟，重置计数
    if now.Sub(req.lastUpdate) > time.Minute {
        req.count = 1
        req.lastUpdate = now
        return false, nil
    }
    
    // 增加计数
    req.count++
    req.lastUpdate = now
    
    // 检查是否超过阈值
    return req.count > threshold, nil
}
```

### 6.6 插件限制和注意事项

#### 允许的标准库包
插件只能导入 Go 标准库，不能使用第三方包。常用的标准库包括：
- 基础：`fmt`, `strings`, `strconv`, `errors`
- 编码：`encoding/json`, `encoding/base64`, `encoding/hex`
- 加密：`crypto/md5`, `crypto/sha256`, `crypto/rand`
- 时间：`time`
- 正则：`regexp`
- 网络：`net`, `net/url`

#### 最佳实践
1. **保持简单**：插件应该专注于单一功能
2. **快速返回**：避免复杂计算，考虑使用缓存
3. **优雅降级**：错误时返回合理的默认值
4. **充分测试**：测试各种边界情况

### 6.7 插件部署和管理

#### 创建插件
1. 在 Web UI 的插件管理页面点击"新建插件"
2. 输入插件名称和代码
3. 系统会自动验证插件语法和安全性
4. 保存后立即可用

#### 测试插件
```xml
<!-- 测试规则 -->
<rule id="test_custom_plugin">
    <check type="PLUGIN">myCustomPlugin(_$test_field, "expected_value")</check>
    <append type="PLUGIN" field="result">myDataPlugin(_$input_data)</append>
</rule>
```

#### 插件版本管理
- 修改插件会创建新版本
- 可以查看插件修改历史
- 支持回滚到之前版本

### 6.8 常见问题解答

#### Q: 如何知道应该使用哪种函数签名？
A: 根据插件的使用场景：
- 在 `<check type="PLUGIN">` 中使用：返回 `(bool, error)`
- 在 `<append type="PLUGIN">` 或 `<plugin>` 中使用：返回 `(interface{}, bool, error)`

#### Q: 插件可以修改输入数据吗？
A: 不可以。插件接收的参数是值传递，修改不会影响原始数据。如需修改数据，应通过返回值实现。

#### Q: 如何在插件之间共享数据？
A: 推荐通过规则引擎的数据流：
1. 第一个插件返回结果到字段
2. 第二个插件从该字段读取数据

#### Q: 插件执行超时怎么办？
A: 系统有默认的超时保护机制。如果插件执行时间过长，会被强制终止并返回错误。

## 🎯 总结

AgentSmith-HUB 规则引擎的核心优势：

1. **完全灵活的执行顺序**：操作按XML中的出现顺序执行
2. **简洁的语法**：独立的 `<check>` 标签，支持灵活组合
3. **强大的数据处理**：丰富的内置插件和灵活的字段访问
4. **可扩展性**：支持自定义插件开发
5. **高性能设计**：智能优化和缓存机制

记住核心理念：**按需组合，灵活编排**。根据你的具体需求，自由组合各种操作，创建最适合的规则。

祝你使用愉快！🚀