# 🛡️ AgentSmith-HUB 完整指南

AgentSmith-HUB 规则引擎是一个强大的实时数据处理引擎，它能够：
 - 🔍 **实时检测**：从数据流中识别威胁和异常
 - 🔄 **数据转换**：对数据进行加工和丰富化
 - 📊 **统计分析**：进行阈值检测和频率分析
 - 📖 **插件支持**：支持自定义插件
 - 🚨 **自动响应**：触发告警和自动化操作

### 核心理念：灵活的执行顺序

规则引擎采用**灵活的执行顺序**，操作按照在XML中的出现顺序执行，让你可以根据具体需求自由组合各种操作。

## 📋 第一部分：核心组件语法

### 1.1 INPUT 语法说明

INPUT 定义了数据输入源，支持多种数据源类型。

#### 支持的数据源类型

##### Kafka 
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
    - "localhost:9093"
  topic: "security_events"
  group: "agentsmith_consumer"
  compression: "snappy"  # 可选：none, snappy, gzip
  # SASL 认证（可选）
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS 配置（可选）
  tls:
    enable: true
    ca_file: "/path/to/ca.pem"
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

##### 阿里云SLS 
```yaml
type: aliyun_sls
aliyun_sls:
  endpoint: "cn-hangzhou.log.aliyuncs.com"
  access_key_id: "YOUR_ACCESS_KEY_ID"
  access_key_secret: "YOUR_ACCESS_KEY_SECRET"
  project: "your_project_name"
  logstore: "your_logstore_name"
  consumer_group_name: "your_consumer_group"
  consumer_name: "your_consumer_name"
  cursor_position: "end"  # begin, end, 或具体时间戳
  cursor_start_time: 1640995200000  # Unix时间戳（毫秒）
  query: "* | where attack_type_name != 'null'"  # 可选的查询过滤条件
```

##### Kafka Azure 
```yaml
type: kafka_azure
kafka:
  brokers:
    - "your-namespace.servicebus.windows.net:9093"
  topic: "your_topic"
  group: "your_consumer_group"
  sasl:
    enable: true
    mechanism: "plain"
    username: "$ConnectionString"
    password: "your_connection_string"
  tls:
    enable: true
```

##### Kafka AWS 
```yaml
type: kafka_aws
kafka:
  brokers:
    - "your-cluster.amazonaws.com:9092"
  topic: "your_topic"
  group: "your_consumer_group"
  sasl:
    enable: true
    mechanism: "aws_msk_iam"
    aws_region: "us-east-1"
  tls:
    enable: true
```

#### Grok 模式支持

INPUT 组件支持 Grok 模式解析日志数据。如果配置了 `grok_pattern`，输入组件将使用指定模式解析 `message` 字段。如果未配置，数据将按 JSON 格式处理。

##### Grok 模式配置
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  topic: "log-topic"
  group: "grok-test-group"
  offset_reset: "earliest"

# Grok 模式解析日志数据
grok_pattern: "%{COMBINEDAPACHELOG}"
```

##### 常用 Grok 模式

**预定义模式：**
- `%{COMBINEDAPACHELOG}` - Apache 组合日志格式
- `%{IP:client} %{WORD:method} %{URIPATHPARAM:request} %{NUMBER:bytes} %{NUMBER:duration}` - 简单 HTTP 日志格式
- `%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} %{GREEDYDATA:message}` - 带 ISO8601 时间戳的标准日志格式

**自定义正则表达式：**
```yaml
# 自定义时间戳格式
grok_pattern: "(?<timestamp>\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z) (?<client_ip>\\d+\\.\\d+\\.\\d+\\.\\d+) (?<method>GET|POST|PUT|DELETE) (?<path>/[a-zA-Z0-9/_-]*)"

# 自定义日志格式
grok_pattern: "(?<timestamp>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) (?<level>\\w+) (?<message>.*)"
```

**数据流程：**
```
输入数据 (map[string]interface{})
↓
检查是否配置了 grok_pattern
↓
如果配置了：解析 message 字段并将结果合并到原始数据中
如果未配置：保持原始数据不变
↓
传递给下游（JSON 格式）
```

### 1.2 OUTPUT 语法说明

OUTPUT 定义了数据处理结果的输出目标。

#### 支持的输出类型

##### Print 输出（控制台打印）
```yaml
type: print
```

##### Kafka 
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
    - "localhost:9093"
  topic: "processed_events"
  key: "user_id"  # 可选：指定消息key字段
  compression: "snappy"  # 可选：none, snappy, gzip
  # SASL 认证（可选）
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS 配置（可选）
  tls:
    enable: true
    ca_file: "/path/to/ca.pem"
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

##### Elasticsearch 
```yaml
type: elasticsearch
elasticsearch:
  hosts:
    - "http://localhost:9200"
    - "https://localhost:9201"
  index: "security-events-{YYYY.MM.DD}"  # 支持时间模式
  batch_size: 1000  # 批量写入大小
  flush_dur: "5s"   # 刷新间隔
  # 认证配置（可选）
  auth:
    type: basic  # basic, api_key, bearer
    username: "elastic"
    password: "password"
    # 或者使用API Key
    # api_key: "your-api-key"
    # 或者使用Bearer Token
    # token: "your-bearer-token"
```


**支持的索引名称时间模式：**
- `{YYYY}` - 完整年份 (例如: 2024)
- `{YY}` - 短年份 (例如: 24)
- `{MM}` - 月份 (例如: 01-12)
- `{DD}` - 日期 (例如: 01-31)
- `{HH}` - 小时 (例如: 00-23)
- `{mm}` - 分钟 (例如: 00-59)
- `{ss}` - 秒 (例如: 00-59)
- `{YYYY.MM.DD}` - 点分隔日期 (例如: 2024.01.15)
- `{YYYY-MM-DD}` - 横线分隔日期 (例如: 2024-01-15)
- `{YYYY/MM/DD}` - 斜线分隔日期 (例如: 2024/01/15)
- `{YYYY_MM_DD}` - 下划线分隔日期 (例如: 2024_01_15)
- `{YYYY.MM}` - 年月点分隔 (例如: 2024.01)
- `{YYYY-MM}` - 年月横线分隔 (例如: 2024-01)
- `{YYYY/MM}` - 年月斜线分隔 (例如: 2024/01)
- `{YYYY_MM}` - 年月下划线分隔 (例如: 2024_01)

**示例：**
```yaml
index: "logs-{YYYY.MM.DD}"        # logs-2024.01.15
index: "events-{YYYY-MM-DD}"      # events-2024-01-15
index: "alerts-{YYYY_MM_DD}"      # alerts-2024_01_15
index: "metrics-{YYYY.MM}"        # metrics-2024.01
index: "hourly-{YYYY.MM.DD}-{HH}" # hourly-2024.01.15-14
```


### 1.3 PROJECT 语法说明

PROJECT 定义了项目的整体配置，使用简单的箭头语法来描述数据流。

#### 基本语法
```yaml
content: |
  INPUT.输入组件名 -> RULESET.规则集名
  RULESET.规则集名 -> OUTPUT.输出组件名
```

#### 项目配置示例

```yaml
content: |
  INPUT.kafka -> RULESET.security_rules
  RULESET.security_rules -> OUTPUT.elasticsearch
```

#### 复杂数据流示例

```yaml
content: |
  # 主数据流
  INPUT.kafka -> RULESET.exclude
  RULESET.exclude -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.compliance_check
  RULESET.compliance_check -> OUTPUT.elasticsearch
  
  # 告警流
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # 日志流
  RULESET.compliance_check -> OUTPUT.print
```

#### 数据流规则说明

**基本规则**：
 - 1.使用 `->` 箭头表示数据流向
 - 2.组件引用格式：`类型.组件名`
 - 3.支持的类型：`INPUT`、`RULESET`、`OUTPUT`
 - 4.每行一个数据流定义
 - 5.支持注释（以 `#` 开头）

**数据流特点**：
 - 1.数据按箭头方向流动
 - 2.一个组件可以有多个下游组件
 - 3.支持分支和合并
 - 4.白名单规则集通常放在最前面

**实际项目示例**：

```yaml
content: |
  # 网络安全监控项目
  # 数据从Kafka流入，经过多层规则处理，最终输出到不同目标
  
  INPUT.security_kafka -> RULESET.exclude
  RULESET.exclude -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.behavior_analysis
  RULESET.behavior_analysis -> OUTPUT.security_es
  
  # 高威胁事件单独告警
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # 调试信息打印
  RULESET.behavior_analysis -> OUTPUT.debug_print
```

## 🔧 第二部分：基本操作指南

### 2.1 临时文件和正式文件

当点击+（新建组件）或者双击组件后（编辑已有组件）会进入针对组件的编辑模式，需要注意在编辑模式下保存（点击 Save 或者使用 Cmd+S 快捷键），都不会直接保存为正式组件，而是会通过临时文件的方式进行保存，如果需要成为真正可以被使用的组件，需要在 Setting -> Push Changes 进行配置 Review 后进行 Apply。

提交变更后 HUB 会自动重启受影响的 Project。

![PushChanges](/png/PushChanges.png)

### 2.2 从本地文件读取配置

组件配置也可以直接放置到 HUB 的 Config 文件夹内，放置后也需要在 Setting -> Load Local Components 进行配置 Review 后进行 Load。

提交变更后 HUB 会自动重启受影响的 Project。

![LoadLocalComponents](/png/LoadLocalComponents.png)

### 2.3 灵活使用测试和查看 Sample Data

Output、Ruleset、Plugin、Project 均支持测试，其中 Project 测试时选择Input数据输入，展示原来需要通过 Output 输出的数据（不会真的流入Output组件），Cmd+D 是测试快捷键，可以快速唤起测试。
![PluginTest.png](/png/PluginTest.png)
![RulesetTest.png](/png/RulesetTest.png)
![ProjectTest.png](/png/ProjectTest.png)

每个运行的组件会采集 Sample Data，我们可以通过组件菜单选择 “View Sample Data” 或者在 Project 流转图中对组件进行右键点击查看 Sample Data。Sample Data 每6分钟采样一条，一共保存100条数据。
![SampleData](/png/SampleData.png)


### 2.4 其他功能

- 所有组件编辑都支持智能补全、提示；
![EditRuleset.png](/png/EditRuleset.png)
- 所有组件编辑的时候都支持语法的 Verify，在 Save 按钮左侧；Input 和 Output 组件支持 Connect Check；
![ConnectCheck.png](/png/ConnectCheck.png)
- 搜索框不仅支持搜索配置名，也支持搜索配置内具体配置；
![Search.png](/png/Search.png)
- Setting 支持查看 HUB 和 Pluin 的报错，在Error Logs 内查看；Setting 的 Operations History 支持查看历史的配置提交、Project 操作、集群内部指令下发等。
![Errors.png](/png/Errors.png)
![OperationsHistory.png](/png/OperationsHistory.png)

### 2.5 MCP

AgentSmith-HUB 支持 MCP，Token 于 Server 共同，以下是 Cline 配置：

```json
{
  "mcpServers": {
    "agentsmith-hub": {
      "disabled": false,
      "timeout": 60,
      "type": "streamableHttp",
      "url": "http://192.168.124.5/mcp",
      "headers": {
         "token": "your-hub-token"
      }
    }
  }
}
```

目前可以通过 MCP 覆盖了大部分使用场景，包括策略编辑等。
![MCP.png](/png/MCP.png)

## 📚 第三部分：RULESET 语法详解

### 3.1 你的第一个规则

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
- 规则引擎从输入数据中提取 `field` 指定的字段值
- 使用 `type` 指定的比较方式，将字段值与标签内容进行比较
- 返回 true 或 false 的检查结果

#### 🔍 语法详解：`<append>` 标签

`<append>` 用于向数据中添加新字段或修改现有字段。

**基本语法：**
```xml
<append field="字段名">要添加的值</append>
```

**属性说明：**
- `field`（必需）：要添加或修改的字段名;
- `type`（可选）：当值为 "PLUGIN" 时，表示使用插件生成值。

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

### 3.2 添加更多检查条件

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
- 默认使用 **AND** 逻辑：所有检查都必须通过，规则才匹配;
- 检查按顺序执行：如果某个检查失败，后续检查不会执行（短路求值）;
- 这种设计提高了性能：尽早失败，避免不必要的检查。

在上面的例子中，三个检查条件必须**全部满足**：
- username 等于 "admin" ;
- login_time 大于 22（晚上10点后）;
- failed_attempts 大于 3。

#### 🔍 语法详解：`<plugin>` 标签

`<plugin>` 用于执行自定义操作，通常用于响应动作。

**基本语法：**
```xml
<plugin>插件名称(参数1, 参数2, ...)</plugin>
```

**特点：**
- 执行操作但不返回值到数据中;
- 通常用于外部动作：发送告警、执行阻断、记录日志等;
- 只在规则匹配成功后执行。

**与 `<append type="PLUGIN">` 的区别：**
- `<plugin>`：执行操作，不返回值;
- `<append type="PLUGIN">`：执行插件并将返回值添加到数据中。

### 3.3 使用动态值

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
            calculate_ratio(amount, user.daily_limit)
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
- `_$字段名`：引用单个字段（插件内使用不需要遵循该语法）;
- `_$父字段.子字段`：引用嵌套字段（插件内使用不需要遵循该语法）;
- `_$ORIDATA`：引用整个原始数据对象（插件内使用也需要遵循该语法）。

**工作原理：**
- 当规则引擎遇到 `_$` 前缀时，会将其识别为动态引用；但是在插件中要应用检测数据内数据时，不需要使用该前缀，直接使用该字段即可；
- 从当前处理的数据中提取对应字段的值；
- 使用提取的值进行比较或处理。

**在上面的例子中：**
- check 中 `_$user.daily_limit` 从数据中提取 `user.daily_limit` 的值（5000）；
- plugin 中 `amount` 提取 `amount` 字段的值（10000）；`user.daily_limit` 从数据中提取 `user.daily_limit` 的值（5000）；
- 动态比较：10000 > 5000，条件满足。

**常见用法：**
```xml
<!-- 动态比较两个字段 -->
<check type="NEQ" field="current_user">login_user</check>

<!-- 在 append 中使用动态值 -->
<append field="username">_$username</append>

<!-- 在插件参数中使用 -->
<plugin>blockIP(malicious_ip, block_duration)</plugin>
```

**_$ORIDATA 的使用：**
`_$ORIDATA` 代表整个原始数据对象，常用于：
- 将完整数据传递给插件进行复杂处理；
- 生成包含所有信息的告警；
- 数据备份或归档。

```xml
<!-- 将整个数据对象发送给分析插件 -->
<append type="PLUGIN" field="risk_analysis">analyzeFullData(_$ORIDATA)</append>

<!-- 生成完整告警 -->
<plugin>sendAlert(_$ORIDATA, "HIGH_RISK")</plugin>
```

## 📊 第四部分：数据处理进阶

### 4.1 灵活的执行顺序

规则引擎的一大特点是灵活的执行顺序：

```xml
<rule id="flexible_way" name="灵活处理示例">
    <!-- 可以先添加时间戳 -->
    <append type="PLUGIN" field="check_time">now()</append>
    
    <!-- 然后进行检查 -->
    <check type="EQU" field="event_type">security_event</check>
    
    <!-- 统计阈值可以放在任何位置 -->
   <threshold group_by="source_ip" range="5m">10</threshold>
    
    <!-- 继续其他检查（假设有自定义插件） -->
    <check type="PLUGIN">is_working_hours(check_time)</check>
    
    <!-- 最后处理 -->
    <append field="processed">true</append>
</rule>
```

#### 💡 重要概念：执行顺序的意义

**为什么执行顺序很重要？**

- **数据增强**：可以先添加字段，然后基于新字段做检查
- **性能优化**：将快速检查放在前面，复杂操作放在后面
- **条件处理**：某些操作可能依赖前面操作的结果

**执行流程：**
- 规则引擎按照 XML 中标签的出现顺序执行操作
- 检查类操作（check、threshold）如果失败，规则立即结束
- 处理类操作（append、del、plugin）只在所有检查通过后执行

#### 🔍 语法详解：`<threshold>` 标签

`<threshold>` 用于检测在指定时间窗口内事件发生的频率。

**基本语法：**
```xml
<threshold group_by="分组字段" range="时间范围">阈值</threshold>
```

**属性说明：**
- `group_by`（必需）：按哪个字段分组统计，可以多个字段用逗号分隔；
- `range`（必需）：时间窗口，支持 s(秒)、m(分)、h(时)、d(天)；
- `value`（必需）：触发阈值，达到这个数量时检查通过。

**工作原理：**
- 按 `group_by` 字段对事件分组（如按 source_ip 分组）；
- 在 `range` 指定的滑动时间窗口内统计每组的事件数量；
- 当某组的统计值达到 `value` 时，该检查通过。

**在上面的例子中：**
- 按 source_ip 分组；
- 统计 5 分钟内的事件数；
- 如果某个 IP 在 5 分钟内触发 10 次，则阈值检查通过。

### 4.2 复杂的嵌套数据处理

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
                   range="1h">3</threshold>
        
        <!-- 使用插件进行深度分析（假设有自定义插件） -->
        <check type="PLUGIN">analyze_transfer_risk(request.body)</check>
        
        <!-- 提取和处理user-agent -->
        <append type="PLUGIN" field="client_info">parseUA(request.headers.user-agent)</append>
        
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
- 使用逗号分隔多个字段；
- 支持嵌套字段路径：`user.password,session.token`；
- 如果字段不存在，不会报错，静默忽略；
- 只在规则匹配成功后执行。

**使用场景：**
- 删除敏感信息（密码、token、密钥等）；
- 清理临时字段；
- 减少数据体积，避免传输不必要的信息。

**在上面的例子中：**
- `request.headers.authorization` 包含敏感的认证信息；
- 使用 `<del>` 在数据处理后删除该字段；
- 确保敏感信息不会被存储或传输。

#### 🔍 语法详解：`<iterator>` 标签

`<iterator>` 用于对数组/列表中的每个元素执行一组检查，支持两种判定方式：`ANY`（任意一个元素匹配即通过）和 `ALL`（所有元素都必须匹配）。

**基本语法：**
```xml
<iterator type="ANY|ALL" field="数组字段路径" variable="迭代变量名">
    <!-- 可包含：check / threshold / checklist -->
    <check ...>...</check>
    <threshold ...>...</threshold>
    <checklist condition="...">
        <check id="..." ...>...</check>
        <!-- 也可包含 threshold -->
    </checklist>
</iterator>
```

**属性说明：**
- `type`（必需）：`ANY` 或 `ALL`
- `field`（必需）：要迭代的数组字段路径；支持：
  - 原生数组：`[]interface{}`、`[]string`、`[]map[string]interface{}`
  - JSON 字符串且内容为数组（会自动解析）
- `variable`（必需）：迭代变量名，用于在内部访问当前元素
  - 命名规则：以字母或下划线开头，只能包含字母、数字、下划线
  - 不可与内部前缀或保留名冲突（如：`_$`、`ORIDATA` 等）

**执行语义：**
- 迭代上下文：在 `<iterator>` 内执行时，默认上下文被替换为仅包含迭代变量的新上下文： `{variable: 当前元素}`。
  - 在内部 `<check>`/`<threshold>`/`<checklist>` 的 `field` 中通过迭代变量访问当前元素，如：`variable.name`、`variable._ip`、`variable.value`。
- `ANY`：任意元素使内部检查整体通过，则迭代返回 true；
- `ALL`：所有元素都满足内部检查，迭代才返回 true。

**示例1：字符串数组（任意一个为公网IP）**
```json
{"ips": ["1.2.3.4", "10.0.0.1", "8.8.8.8"]}
```
```xml
<rule id="ip_any_public" name="任意公网IP">
    <iterator type="ANY" field="ips" variable="_ip">
        <check type="PLUGIN">!isPrivateIP(_ip)</check>
    </iterator>
</rule>
```

**示例2：对象数组（全部进程都不为浏览器）**
```json
{"processes": [{"name":"powershell.exe","cmd":"..."},{"name":"svchost.exe","cmd":"..."}]}
```
```xml
<rule id="no_browser" name="进程白名单">
    <iterator type="ALL" field="processes" variable="proc">
        <check type="NI" field="proc.name" logic="AND" delimiter="|">chrome.exe|firefox.exe</check>
        <checklist condition="a or b">
            <check id="a" type="INCL" field="proc.name">powershell</check>
            <check id="b" type="INCL" field="proc.cmd">-EncodedCommand</check>
        </checklist>
    </iterator>
</rule>
```

**示例3：JSON 字符串数组（全部以 .com 结尾）**
```json
{"targets": "[\"example.com\", \"test.com\"]"}
```
```xml
<rule id="domains_all_com" name="域名后缀检查">
    <iterator type="ALL" field="targets" variable="_domain">
        <check type="END" field="_domain">.com</check>
    </iterator>
</rule>
```
### 4.3 条件组合逻辑（checklist）

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
                is_known_malware(hash)
            </check>
</checklist>
        
        <!-- 丰富化数据 -->
        <append type="PLUGIN" field="virus_scan">virusTotal(hash)</append>
        <append field="threat_level">high</append>
        
        <!-- 自动响应（假设有自定义插件） -->
        <plugin>quarantine_file(filename)</plugin>
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
- 使用 `and`、`or` 连接条件；
- 使用 `()` 分组，控制优先级；
- 使用 `not` 取反；
- 只能使用小写的逻辑操作符。

**示例表达式：**
- `a and b and c`：所有条件都满足；
- `a or b or c`：任一条件满足；
- `(a or b) and not c`：a或b满足，且c不满足；
- `a and (b or (c and d))`：复杂嵌套条件。

**在checklist中使用threshold的示例：**
```xml
<checklist condition="suspicious_activity and high_frequency">
    <check id="suspicious_activity" type="INCL" field="command">powershell|cmd|wmic</check>
    <threshold id="high_frequency" group_by="source_ip" range="5m">10</threshold>
</checklist>
```
- 检查命令是否包含可疑关键词
- 同时检查源IP在5分钟内是否触发超过10次
- 两个条件都满足时，checklist通过

**工作原理：**
- 执行所有带 `id` 的检查节点和阈值节点，记录每个节点的结果（true/false）
- 将结果代入 `condition` 表达式计算最终结果
- 如果最终结果为 true，则 checklist 通过

**支持节点类型：**
- `<check>` 节点：执行字段检查、正则匹配、插件调用等
- `<threshold>` 节点：执行阈值检测，支持计数、求和、分类统计等模式

#### 🔍 语法详解：多值匹配（logic 和 delimiter）

当需要检查一个字段是否匹配多个值时，可以使用多值匹配语法。

**基本语法：**
```xml
<check type="类型" field="字段" logic="OR|AND" delimiter="分隔符">
    值1分隔符值2分隔符值3
</check>
```

**属性说明：**
- `logic`："OR" 或 "AND"，指定多个值之间的逻辑关系；
- `delimiter`：分隔符，用于分割多个值。

**工作原理：**
- 使用 `delimiter` 将标签内容分割成多个值
- 对每个值分别进行检查
- 根据 `logic` 决定最终结果：
   - `logic="OR"`：任一值匹配即返回 true
   - `logic="AND"`：所有值都匹配才返回 true

**在上面的例子中：**
```xml
<check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
    .exe|.dll|.scr|.bat
</check>
```
- 检查 filename 是否包含 .exe、.dll、.scr 或 .bat；
- 使用 OR 逻辑：任一扩展名匹配即可；
- 使用 | 作为分隔符。

## 🔧 第五部分：高级特性详解

### 5.1 阈值检测的三种模式

`<threshold>` 标签不仅可以简单计数，还支持三种强大的统计模式：

- **默认模式（计数）**：统计事件发生次数
- **SUM 模式**：对指定字段求和
- **CLASSIFY 模式**：统计不同值的数量（去重计数）

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
   <threshold group_by="user,ip" range="5m">5</threshold>
    
    <append field="alert_type">brute_force_attempt</append>
    <plugin>block_ip(ip, 3600)</plugin>  <!-- 封禁1小时 -->
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
               count_field="amount">50000</threshold>
    
    <append field="action">freeze_account</append>
</rule>
```

#### 🔍 高级语法：threshold 的 SUM 模式

**属性说明：**
- `count_type="SUM"`：启用求和模式；
- `count_field`（必需）：要求和的字段名；
- `value`：当累计和达到此值时触发。

**工作原理：**
- 按 `group_by` 分组
- 在时间窗口内累加 `count_field` 的值
- 当累计值达到 `value` 时触发

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
               count_field="file_id">25</threshold>
    
    <append field="risk_score">high</append>
    <plugin>alert_dlp_team(_$ORIDATA)</plugin>
</rule>
```

#### 🔍 高级语法：threshold 的 CLASSIFY 模式

**属性说明：**
- `count_type="CLASSIFY"`：启用去重计数模式；
- `count_field`（必需）：要统计不同值的字段；
- `value`：当不同值数量达到此值时触发。

**工作原理：**
- 按 `group_by` 分组
- 在时间窗口内收集 `count_field` 的所有不同值
- 当不同值的数量达到 `value` 时触发

**使用场景：**
- 检测扫描行为（访问多个不同端口/IP）；
- 数据外泄检测（访问多个不同文件）；
- 异常行为检测（使用多个不同账号）。

### 5.2 内置插件系统

AgentSmith-HUB 提供了丰富的内置插件，无需额外开发即可使用。

#### 🧩 内置插件完整列表

##### 检查类插件（返回bool）

| 插件名 | 功能 | 参数 | 示例 |
|--------|------|------|------|
| `isPrivateIP` | 检查IP是否为私有地址 | ip (string) | `isPrivateIP(source_ip)` |
| `cidrMatch` | 检查IP是否在CIDR范围内 | ip (string), cidr (string) | `cidrMatch(client_ip, "192.168.1.0/24")` |
| `geoMatch` | 检查IP所属国家 | ip (string), countryISO (string) | `geoMatch(source_ip, "US")` |
| `suppressOnce` | 告警抑制 | key (any), windowSec (int), ruleid (string, optional) | `suppressOnce(alert_key, 300, "rule_001")` |

##### 数据处理插件（返回各种类型）

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

**注意插件参数格式**：
- 当引用数据中的字段时，无需使用 `_$` 前缀，直接使用字段名：`source_ip`
- 当完整引用全部原始数据时：`_$ORIDATA`
- 当使用静态值时，直接使用字符串（带引号）：`"192.168.1.0/24"`
- 当使用数字时，不需要引号：`300`

## 第六部分：Ruleset 最佳实践

### 6.1 复杂逻辑组合

```xml
<rule id="complex_plugin_usage" name="Complex Plugin Usage">
    <!-- 使用checklist组合多个条件 -->
    <checklist condition="(private_ip or suspicious_country) and not excluded">
  <check id="private_ip" type="PLUGIN">isPrivateIP(source_ip)</check>
  <check id="suspicious_country" type="PLUGIN">geoMatch(source_ip, "CN")</check>
  <check id="excluded" type="PLUGIN">cidrMatch(source_ip, "10.0.0.0/8")</check>
</checklist>
    
    <!-- 数据富化 -->
    <append type="PLUGIN" field="threat_intel">virusTotal(file_hash)</append>
    <append type="PLUGIN" field="geo_info">shodan(source_ip)</append>
    
    <!-- 时间相关处理 -->
    <append type="PLUGIN" field="hour">hourOfDay()</append>
    <check type="PLUGIN">hourOfDay() > 22</check>
</rule>
```

### 6.2 告警抑制示例

```xml
<rule id="suppression_example" name="Alert Suppression">
    <check type="EQU" field="event_type">login_failed</check>
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_brute_force")</check>
    <append field="alert_type">brute_force</append>
</rule>
```

### 6.3 数据转换示例

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
    <check type="PLUGIN">isPrivateIP(source_ip)</check>  <!-- 源是内网 -->
    <check type="PLUGIN">!isPrivateIP(dest_ip)</check>  <!-- 目标是外网 -->
    
    <!-- 检查地理位置 -->
    <append type="PLUGIN" field="dest_country">geoMatch(dest_ip)</append>
    
    <!-- 添加时间戳 -->
    <append type="PLUGIN" field="detection_time">now()</append>
    <append type="PLUGIN" field="detection_hour">hourOfDay()</append>
    
    <!-- 计算数据外泄风险 -->
    <check type="MT" field="bytes_sent">1000000</check>  <!-- 大于1MB -->
    
    <!-- 生成告警 -->
    <append field="alert_type">potential_data_exfiltration</append>
    
    <!-- 查询威胁情报（如果有配置） -->
    <append type="PLUGIN" field="threat_intel">threatBook(dest_ip, "ip")</append>
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
    <check type="PLUGIN">!isPrivateIP(dest_ip)</check>

    <!-- 第3步：查询威胁情报，增强数据 -->
    <append type="PLUGIN" field="threat_intel">threatBook(dest_ip, "ip")</append>
    
    <!-- 第4步：解析威胁情报结果 -->
    <append type="PLUGIN" field="threat_level">
        parseJSON(threat_intel)
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
        parseJSON(threat_intel.reputation_score)
    </append>
    <append type="PLUGIN" field="threat_tags">
        parseJSON(threat_intel.tags)
    </append>
    
    <!-- 第7步：生成详细告警（假设有自定义插件） -->
    <plugin>generateThreatAlert(_$ORIDATA, threat_intel)</plugin>
</rule>
```

#### 💡 关键优势展示

这个示例展示了灵活执行顺序的几个关键优势：

- **性能优化**：先执行快速检查（datatype），避免对所有数据查询威胁情报；
- **逐步增强**：先确认是公网IP，再查询威胁情报，避免无效查询；
- **动态决策**：基于威胁情报的返回结果动态调整后续处理；
- **条件响应**：只对高威胁等级执行响应操作；
- **数据利用**：充分利用威胁情报返回的丰富数据。

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
    <append type="PLUGIN" field="parsed_body">parseJSON(request_body)</append>
    
    <!-- 解析User-Agent -->
    <append type="PLUGIN" field="browser_info">parseUA(user_agent)</append>
    
    <!-- 提取错误信息 -->
    <append type="PLUGIN" field="error_type">
        regexExtract(stack_trace, "([A-Za-z.]+Exception)")
    </append>
    
    <!-- 时间处理 -->
    <append type="PLUGIN" field="readable_time">tsToDate(timestamp)</append>
    <append type="PLUGIN" field="hour">hourOfDay(timestamp)</append>
    
    <!-- 数据脱敏 -->
    <append type="PLUGIN" field="sanitized_message">
        regexReplace(message, "password\":\"[^\"]+", "password\":\"***")
    </append>
    
    <!-- 告警抑制：同类错误5分钟只报一次 -->
    <check type="PLUGIN">suppressOnce(error_type, 300, "error_log_analysis")</check>
    
    <!-- 生成告警（假设有自定义插件） -->
    <plugin>sendToElasticsearch(_$ORIDATA)</plugin>
</rule>
```

##### 数据脱敏和安全处理

```xml
<rule id="data_masking" name="数据脱敏处理">
    <check type="EQU" field="contains_sensitive_data">true</check>
    
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

#### ⚠️ 告警抑制最佳实践（suppressOnce）

告警抑制插件可以防止同一告警在短时间内重复触发。

**为什么需要 ruleid 参数？**

如果不使用 `ruleid` 参数，不同规则对同一key的抑制会相互影响：

```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(source_ip, 300)</check>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(source_ip, 300)</check>
</rule>
```

**问题**：规则A触发后，规则B对同一IP也会被抑制！

**正确用法**：使用 `ruleid` 参数隔离不同规则：

```xml
<!-- 规则A：网络威胁检测 -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(source_ip, 300, "network_threat")</check>
</rule>

<!-- 规则B：登录异常检测 -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_anomaly")</check>
</rule>
```

### 6.4 排除规则集

排除用于过滤掉不需要处理的数据（ruleset type 为 EXCLUDE）。排除的特殊行为：
- 当排除规则匹配时，数据被"不允许通过"（即被过滤掉，不再继续处理，数据将被丢弃）；
- 当所有排除规则都不匹配时，数据继续传递给后续处理。

```xml
<root type="EXCLUDE" name="security_exclude" author="security_team">
    <!-- 排除规则1：信任的IP -->
    <rule id="trusted_ips">
        <check type="INCL" field="source_ip" logic="OR" delimiter="|">
            10.0.0.1|10.0.0.2|10.0.0.3
        </check>
        <append field="excluded">true</append>
    </rule>
    
    <!-- 排除规则2：已知的良性进程 -->
    <rule id="benign_processes">
        <check type="INCL" field="process_name" logic="OR" delimiter="|">
            chrome.exe|firefox.exe|explorer.exe
        </check>
        <!-- 可以添加多个检查条件，全部满足才会被排除过滤 -->
        <check type="PLUGIN">isPrivateIP(source_ip)</check>
    </rule>
    
    <!-- 排除规则3：内部测试流量 -->
    <rule id="test_traffic">
        <check type="INCL" field="user_agent">internal-testing-bot</check>
        <check type="PLUGIN">cidrMatch(source_ip, "192.168.100.0/24")</check>
    </rule>
</root>
```

## 🚨 第七部分：实战案例集

### 7.1 案例1：APT攻击检测

完整的APT攻击检测规则集（使用内置插件和假设的自定义插件）：

```xml
<root type="DETECTION" name="apt_detection_suite" author="threat_hunting_team">
    <!-- 规则1：PowerShell Empire检测 -->
    <rule id="powershell_empire" name="PowerShell Empire C2检测">
        <!-- 灵活顺序：先enrichment再检测 -->
        <append type="PLUGIN" field="decoded_cmd">base64Decode(command_line)</append>
        
        <!-- 检查进程名 -->
        <check type="INCL" field="process_name">powershell</check>
        
        <!-- 检测Empire特征 -->
        <check type="INCL" field="decoded_cmd" logic="OR" delimiter="|">
            System.Net.WebClient|DownloadString|IEX|Invoke-Expression
        </check>
        
        <!-- 检测编码命令 -->
        <check type="INCL" field="command_line">-EncodedCommand</check>
        
        <!-- 网络连接检测 -->
       <threshold group_by="hostname" range="10m">3</threshold>
        
        <!-- 威胁情报查询 -->
        <append type="PLUGIN" field="c2_url">
            regexExtract(decoded_cmd, "https?://[^\\s]+")
        </append>
        
        <!-- 生成IoC -->
        <append field="ioc_type">powershell_empire_c2</append>
        <append type="PLUGIN" field="ioc_hash">hashSHA256(decoded_cmd)</append>
        
        <!-- 自动响应（假设有自定义插件） -->
        <plugin>isolateHost(hostname)</plugin>
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
                isPrivateIP(source_ip)
            </check>
</checklist>
        
        <!-- 时间窗口检测 -->
       <threshold group_by="source_ip,dest_ip" range="30m">5</threshold>
        
        <!-- 风险评分（假设有自定义插件） -->
        <append type="PLUGIN" field="risk_score">
            calculateLateralMovementRisk(ORIDATA)
        </append>
        
        <plugin>updateAttackGraph(source_ip, dest_ip)</plugin>
    </rule>
    
    <!-- 规则3：数据外泄检测 -->
    <rule id="data_exfiltration" name="数据外泄检测">
        <!-- 先检查是否为敏感数据访问 -->
        <check type="INCL" field="file_path" logic="OR" delimiter="|">
            /etc/passwd|/etc/shadow|.ssh/|.aws/credentials
        </check>

       <!-- 检查外联行为 -->
       <check type="PLUGIN">!isPrivateIP(dest_ip)</check>
       
        <!-- 异常传输检测 -->
        <threshold group_by="source_ip" range="1h" count_type="SUM" 
                   count_field="bytes_sent">1073741824</threshold>  <!-- 1GB -->
        
        <!-- DNS隧道检测（假设有自定义插件） -->
        <checklist condition="dns_tunnel_check">
            <check id="dns_tunnel_check" type="PLUGIN">
                detectDNSTunnel(dns_queries)
            </check>
        </checklist>
        
        <!-- 生成告警 -->
        <append field="alert_severity">critical</append>
        <append type="PLUGIN" field="data_classification">
            classifyData(file_path)
        </append>
        
        <plugin>blockDataTransfer(source_ip, dest_ip)</plugin>
        <plugin>triggerIncidentResponse(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 7.2 案例2：金融欺诈实时检测

```xml
<root type="DETECTION" name="fraud_detection_system" author="risk_team">
    <!-- 规则1：账户接管检测 -->
    <rule id="account_takeover" name="账户接管检测">
        <!-- 实时设备指纹（假设有自定义插件） -->
        <append type="PLUGIN" field="device_fingerprint">
            generateFingerprint(user_agent, screen_resolution, timezone)
        </append>
        
        <!-- 检查设备变更（假设有自定义插件） -->
        <check type="PLUGIN">
            isNewDevice(user_id, device_fingerprint)
        </check>
        
        <!-- 地理位置异常（假设有自定义插件） -->
        <append type="PLUGIN" field="geo_distance">
            calculateGeoDistance(user_id, current_ip, last_ip)
        </append>
        <check type="MT" field="geo_distance">500</check>  <!-- 500km -->
        
        <!-- 行为模式分析（假设有自定义插件） -->
        <append type="PLUGIN" field="behavior_score">
            analyzeBehaviorPattern(user_id, recent_actions)
        </append>
        
        <!-- 交易速度检测 -->
       <threshold group_by="user_id" range="10m">5</threshold>
        
        <!-- 风险决策（假设有自定义插件） -->
        <append type="PLUGIN" field="risk_decision">
            makeRiskDecision(behavior_score, geo_distance, device_fingerprint)
        </append>
        
        <!-- 实时干预（假设有自定义插件） -->
        <plugin>requireMFA(user_id, transaction_id)</plugin>
        <plugin>notifyUser(user_id, "suspicious_activity")</plugin>
    </rule>
    
    <!-- 规则2：洗钱行为检测 -->
    <rule id="money_laundering" name="洗钱行为检测">
        <!-- 分散-聚合模式（假设有自定义插件） -->
        <checklist condition="structuring or layering or integration">
            <!-- 结构化拆分 -->
            <check id="structuring" type="PLUGIN">
                detectStructuring(user_id, transaction_history)
            </check>
            <!-- 分层处理 -->
            <check id="layering" type="PLUGIN">
                detectLayering(account_network, transaction_flow)
            </check>
            <!-- 整合阶段 -->
            <check id="integration" type="PLUGIN">
                detectIntegration(merchant_category, transaction_pattern)
            </check>
        </checklist>
        
        <!-- 关联分析（假设有自定义插件） -->
        <append type="PLUGIN" field="network_risk">
            analyzeAccountNetwork(user_id, connected_accounts)
        </append>
        
        <!-- 累计金额监控 -->
        <threshold group_by="account_cluster" range="7d" count_type="SUM"
                   count_field="amount">1000000</threshold>
        
        <!-- 合规报告（假设有自定义插件） -->
        <append type="PLUGIN" field="sar_report">
            generateSAR(_$ORIDATA)  <!-- Suspicious Activity Report -->
        </append>
        
        <plugin>freezeAccountCluster(account_cluster)</plugin>
        <plugin>notifyCompliance(sar_report)</plugin>
    </rule>
</root>
```

### 7.3 案例3：零信任安全架构

```xml
<root type="DETECTION" name="zero_trust_security" author="security_architect">
    <!-- 规则1：持续身份验证 -->
    <rule id="continuous_auth" name="持续身份验证">
        <!-- 每次请求都验证 -->
        <check type="NOTNULL" field="auth_token"></check>
        
        <!-- 验证token有效性（假设有自定义插件） -->
        <check type="PLUGIN">validateToken(auth_token)</check>
        
        <!-- 上下文感知（假设有自定义插件） -->
        <append type="PLUGIN" field="trust_score">
            calculateTrustScore(
                user_id,
                device_trust,
                network_location,
                behavior_baseline,
                time_of_access
            )
        </append>
        
        <!-- 动态权限调整 -->
        <checklist condition="low_trust or anomaly_detected">
            <check id="low_trust" type="LT" field="trust_score">0.7</check>
            <check id="anomaly_detected" type="PLUGIN">
                detectAnomaly(current_behavior, baseline_behavior)
            </check>
    </checklist>
        
        <!-- 微分段策略（假设有自定义插件） -->
        <append type="PLUGIN" field="allowed_resources">
            applyMicroSegmentation(trust_score, requested_resource)
        </append>
        
        <!-- 实时策略执行（假设有自定义插件） -->
        <plugin>enforcePolicy(user_id, allowed_resources)</plugin>
        <plugin>logZeroTrustDecision(_$ORIDATA)</plugin>
</rule>
    
    <!-- 规则2：设备信任评估 -->
    <rule id="device_trust" name="设备信任评估">
        <!-- 设备健康检查（假设有自定义插件） -->
        <append type="PLUGIN" field="device_health">
            checkDeviceHealth(device_id)
        </append>
        
        <!-- 合规性验证（假设有自定义插件） -->
        <checklist condition="patch_level and antivirus and encryption and mdm_enrolled">
            <check id="patch_level" type="PLUGIN">
                isPatchCurrent(os_version, patch_level)
            </check>
            <check id="antivirus" type="PLUGIN">
                isAntivirusActive(av_status)
            </check>
            <check id="encryption" type="PLUGIN">
                isDiskEncrypted(device_id)
            </check>
            <check id="mdm_enrolled" type="PLUGIN">
                isMDMEnrolled(device_id)
            </check>
    </checklist>
        
        <!-- 证书验证（假设有自定义插件） -->
        <check type="PLUGIN">
            validateDeviceCertificate(device_cert)
        </check>
        
        <!-- 信任评分（假设有自定义插件） -->
        <append type="PLUGIN" field="device_trust_score">
            calculateDeviceTrust(_$ORIDATA)
        </append>
        
        <!-- 访问决策（假设有自定义插件） -->
        <plugin>applyDevicePolicy(device_id, device_trust_score)</plugin>
</rule>
</root>
```

## 📖 第八部分：语法参考手册

### 8.1 规则集结构

#### 根元素 `<root>`
```xml
<root type="DETECTION|EXCLUDE" name="规则集名称" author="作者">
    <!-- 规则列表 -->
</root>
```

| 属性 | 必需 | 说明                                           | 默认值 |
|------|------|----------------------------------------------|--------|
| type | 否 | 规则集类型，DETECTION 类型为命中向后传递，EXCLUDE 为命中不向后传递 | DETECTION |
| name | 否 | 规则集名称                                        | - |
| author | 否 | 作者信息                                         | - |

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

#### 多个规则的关系

当一个规则集包含多个 `<rule>` 元素时，它们具有 **OR关系**：

**核心概念：**
- **独立评估**：每个规则都独立地对输入数据进行评估；
- **OR逻辑**：如果任何一个规则匹配，就会生成一条数据记录并向下游传递；
- **多重匹配**：多个规则可以匹配同一条数据，生成多条记录；
- **无顺序依赖**：规则之间不依赖彼此的处理结果。

**执行流程：**
- **并行评估**：规则集中的所有规则都对相同的输入数据进行评估；
- **匹配检测**：每个匹配的规则都会生成一条独立的数据记录；
- **数据生成**：每个匹配的规则都会创建自己的输出，包含特定的数据增强；
- **下游流动**：所有生成的记录都会传递给数据流中的下一个组件。

**示例：**
```xml
<root type="DETECTION" name="多规则示例">
    <!-- 规则1：检测管理员登录 -->
    <rule id="admin_login">
        <check type="EQU" field="username">admin</check>
        <append field="alert_type">admin_login</append>
        <append field="severity">high</append>
    </rule>
    
    <!-- 规则2：检测登录失败 -->
    <rule id="failed_login">
        <check type="EQU" field="result">failure</check>
        <append field="alert_type">failed_login</append>
        <append field="severity">medium</append>
    </rule>
    
    <!-- 规则3：检测异常时间访问 -->
    <rule id="unusual_time">
        <check type="MT" field="hour">22</check>
        <append field="alert_type">unusual_time</append>
        <append field="severity">low</append>
    </rule>
</root>
```

**输入数据：**
```json
{"username": "admin", "result": "success", "hour": 23}
```

**输出：**
- 规则1匹配 → 生成：`{"username": "admin", "result": "success", "hour": 23, "alert_type": "admin_login", "severity": "high"}`
- 规则3匹配 → 生成：`{"username": "admin", "result": "success", "hour": 23, "alert_type": "unusual_time", "severity": "low"}`

**关键要点：**
- **独立处理**：每个规则都独立处理原始输入数据；
- **多重输出**：一条输入可以生成多条输出记录；
- **无数据共享**：规则之间无法共享数据修改；
- **性能**：所有规则都会被评估，因此规则顺序不影响性能。

### 8.2 检查操作

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

### 8.3 检查类型完整列表

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
| PLUGIN | 插件函数（支持 `!` 取反） | `<check type="PLUGIN">isValidEmail(email)</check>` |

### 8.4 频率检测

#### 阈值检测 `<threshold>`
```xml
<threshold group_by="字段1,字段2" range="时间范围"
           count_type="SUM|CLASSIFY" count_field="统计字段" local_cache="true|false">阈值</threshold>
```

| 属性 | 必需 | 说明 | 示例 |
|------|------|------|------|
| group_by | 是 | 分组字段 | `source_ip,user_id` |
| range | 是 | 时间范围 | `5m`, `1h`, `24h` |
| value | 是 | 阈值 | `10` |
| count_type | 否 | 计数类型 | 默认：计数，`SUM`：求和，`CLASSIFY`：去重计数 |
| count_field | 条件 | 统计字段 | 使用SUM/CLASSIFY时必需 |
| local_cache | 否 | 使用本地缓存 | `true` 或 `false` |

### 8.5 数据处理操作

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

### 8.6 字段访问语法

#### 基本访问
- **直接字段**：`field_name`
- **嵌套字段**：`parent.child.grandchild`
- **数组索引**：`array.#0.field`（访问第一个元素）

#### 动态引用（_$前缀）
- **字段引用**：`_$field_name`
- **嵌套引用**：`_$parent.child.field`
- **原始数据**：`_$ORIDATA`
- **数组索引**：`array.#0.field`（访问第一个元素）

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

### 8.7 性能优化建议

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
<threshold group_by="user_id" range="5m" local_cache="true">10</threshold>

<!-- 避免过大的时间窗口 -->
<threshold group_by="ip" range="1h">1000</threshold>  <!-- 不要超过24h -->
```

### 8.8 常见错误和解决方案

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

### 8.9 调试技巧

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

### 8.10 迭代器 `<iterator>`

#### 基本语法
```xml
<iterator type="ANY|ALL" field="数组字段路径" variable="迭代变量名">
    <!-- 内部可包含：check / threshold / checklist -->
    ...
</iterator>
```

#### 属性
| 属性 | 必需 | 说明 |
|------|------|------|
| type | 是 | 迭代判定方式：`ANY` 任意一个元素匹配即通过；`ALL` 所有元素匹配才通过 |
| field | 是 | 要迭代的数组字段路径，支持原生数组或 JSON 字符串数组 |
| variable | 是 | 迭代变量名；以字母或下划线开头，只能包含字母、数字、下划线；不可与内部前缀或保留名（如 `_$`、`ORIDATA`）冲突 |

#### 迭代上下文与字段访问
- 在迭代体内，默认上下文仅包含 `{variable: 当前元素}`；
- 在子节点（`<check>`/`<threshold>`/`<checklist>`）中通过迭代变量访问当前元素：
  - 当前元素为对象：`proc.name`、`item.value`
  - 当前元素为标量（如字符串）：直接使用变量名：`_ip`

#### 数据类型支持
- `[]interface{}`、`[]string`、`[]map[string]interface{}`
- 字符串类型但内容为 JSON 数组（自动解析）

#### 结果判定
- `ANY`：找到任意一个元素使内部检查整体通过 → 返回 true；
- `ALL`：全部元素都通过内部检查 → 返回 true；否则返回 false。

#### 示例
```xml
<iterator type="ANY" field="ips" variable="_ip">
    <check type="PLUGIN">!isPrivateIP(_ip)</check>
</iterator>
```

## 第九部分：自定义插件开发

### 9.1 插件分类

#### 按运行方式分类
- **本地插件（Local Plugin）**：编译到程序中的内置插件，性能最高
- **Yaegi插件（Yaegi Plugin）**：使用Yaegi解释器运行的动态插件，**支持有状态和init函数**

#### 按返回类型分类
- **检查类插件（Check Node Plugin）**：返回 `(bool, error)`，用于 `<check type="PLUGIN">`、`<append type="PLUGIN">` 和 `<plugin>` 中
- **数据处理插件（Other Plugin）**：返回 `(interface{}, bool, error)`，用于 `<append type="PLUGIN">` 和 `<plugin>` 中，如果第二个参数返回false，那么将不会执行 Append 动作

### 9.2 插件语法

#### 基本语法
```xml
<!-- 检查类插件 -->
<check type="PLUGIN">pluginName(param1, param2, ...)</check>

<!-- 数据处理插件 -->
<append type="PLUGIN" field="field_name">pluginName(param1, param2, ...)</append>

<!-- 执行操作插件 -->
<plugin>pluginName(param1, param2, ...)</plugin>
```

#### 参数类型
- **字符串**：`"value"` 或 `'value'`
- **数字**：`123` 或 `123.45`
- **布尔值**：`true` 或 `false`
- **字段引用**：`field_name` 或 `parent.child.field`
- **原始数据**：`_$ORIDATA`（唯一需要_$前缀的）

#### 否定语法
检查类插件支持否定前缀：
```xml
<check type="PLUGIN">!isPrivateIP(source_ip)</check>
```

### 9.3 插件函数签名

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

### 9.4 Yaegi插件的有状态特性

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

### 9.5 插件限制
- 只能使用Go标准库，不能使用第三方包；
- 必须定义名为`Eval`的函数，package 必须为 plugin；
- 函数返回值必须严格匹配要求。

## 总结

记住核心理念：**按需组合，灵活编排**。根据你的具体需求，自由组合各种操作，创建最适合的规则。

祝你使用愉快！🚀
