# 🛡️ AgentSmith-HUB Complete Guide

AgentSmith-HUB Rules Engine is a powerful real-time data processing engine that can:
- 🔍 **Real-time Detection**: Identify threats and anomalies from data streams
- 🔄 **Data Transformation**: Process and enrich data
- 📊 **Statistical Analysis**: Perform threshold detection and frequency analysis
- 📖 **Plugin Support**: Support custom plugins
- 🚨 **Automatic Response**: Trigger alerts and automated operations

### Core Philosophy: Flexible Execution Order

The rules engine adopts a **flexible execution order**, where operations are executed according to their appearance order in XML, allowing you to freely combine various operations based on specific requirements.

## 📋 Part 1: Core Component Syntax

### 1.1 INPUT Syntax Description

INPUT defines data input sources and supports multiple data source types.

#### Supported Data Source Types

##### Kafka 
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
    - "localhost:9093"
  topic: "security_events"
  group: "agentsmith_consumer"
  compression: "snappy"  # Optional: none, snappy, gzip
  # SASL Authentication (optional)
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS Configuration (optional)
  tls:
    enable: true
    ca_file: "/path/to/ca.pem"
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

##### Alibaba Cloud SLS 
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
  cursor_position: "end"  # begin, end, or specific timestamp
  cursor_start_time: 1640995200000  # Unix timestamp (milliseconds)
  query: "* | where attack_type_name != 'null'"  # Optional query filter conditions
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

#### Grok Pattern Support

INPUT components support Grok pattern parsing for log data. If `grok_pattern` is configured, the input will parse the field specified by `grok_field`; if `grok_field` is not set, the `message` field will be parsed by default. If `grok_pattern` is not configured, data will be treated as JSON by default.

##### Grok Pattern Configuration
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  topic: "log-topic"
  group: "grok-test-group"
  offset_reset: "earliest"

# Grok pattern for parsing log data
grok_pattern: "%{COMBINEDAPACHELOG}"
grok_field: content  # Optional: which field to parse; defaults to "message" if unset
```

##### Common Grok Patterns

**Predefined Patterns:**
- `%{COMBINEDAPACHELOG}` - Apache combined log format
- `%{IP:client} %{WORD:method} %{URIPATHPARAM:request} %{NUMBER:bytes} %{NUMBER:duration}` - Simple HTTP log format
- `%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} %{GREEDYDATA:message}` - Standard log format with ISO8601 timestamp

**Custom Regex Patterns:**
```yaml
# Custom timestamp format
grok_pattern: "(?<timestamp>\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z) (?<client_ip>\\d+\\.\\d+\\.\\d+\\.\\d+) (?<method>GET|POST|PUT|DELETE) (?<path>/[a-zA-Z0-9/_-]*)"

# Custom log format
grok_pattern: "(?<timestamp>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) (?<level>\\w+) (?<message>.*)"
```

**Data Flow:**
```
Input Data (map[string]interface{})
↓
Check if grok_pattern is configured
↓
If configured: Parse target field (grok_field if set, otherwise message) and merge results into original data
If not configured: Keep original data unchanged
↓
Pass to downstream (JSON format)
```

### 1.2 OUTPUT Syntax Description

OUTPUT defines the output target for data processing results.

#### Supported Output Types

##### Print Output (Console Print)
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
  key: "user_id"  # Optional: specify message key field
  compression: "snappy"  # Optional: none, snappy, gzip
  # SASL Authentication (optional)
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS Configuration (optional)
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
  index: "security-events-{YYYY.MM.DD}"  # Supports time patterns
  batch_size: 1000  # Batch write size
  flush_dur: "5s"   # Flush interval
  # Authentication configuration (optional)
  auth:
    type: basic  # basic, api_key, bearer
    username: "elastic"
    password: "password"
    # Or use API Key
    # api_key: "your-api-key"
    # Or use Bearer Token
    # token: "your-bearer-token"
```

**Supported Time Patterns for Index Names:**
- `{YYYY}` - Full year (e.g., 2024)
- `{YY}` - Short year (e.g., 24)
- `{MM}` - Month (e.g., 01-12)
- `{DD}` - Day (e.g., 01-31)
- `{HH}` - Hour (e.g., 00-23)
- `{mm}` - Minute (e.g., 00-59)
- `{ss}` - Second (e.g., 00-59)
- `{YYYY.MM.DD}` - Date with dots (e.g., 2024.01.15)
- `{YYYY-MM-DD}` - Date with dashes (e.g., 2024-01-15)
- `{YYYY/MM/DD}` - Date with slashes (e.g., 2024/01/15)
- `{YYYY_MM_DD}` - Date with underscores (e.g., 2024_01_15)
- `{YYYY.MM}` - Year and month with dots (e.g., 2024.01)
- `{YYYY-MM}` - Year and month with dashes (e.g., 2024-01)
- `{YYYY/MM}` - Year and month with slashes (e.g., 2024/01)
- `{YYYY_MM}` - Year and month with underscores (e.g., 2024_01)

**Examples:**
```yaml
index: "logs-{YYYY.MM.DD}"        # logs-2024.01.15
index: "events-{YYYY-MM-DD}"      # events-2024-01-15
index: "alerts-{YYYY_MM_DD}"      # alerts-2024_01_15
index: "metrics-{YYYY.MM}"        # metrics-2024.01
index: "hourly-{YYYY.MM.DD}-{HH}" # hourly-2024.01.15-14
```

### 1.3 PROJECT Syntax Description

PROJECT defines the overall configuration of a project using simple arrow syntax to describe data flow.

#### Basic Syntax
```yaml
content: |
  INPUT.input_component_name -> RULESET.ruleset_name
  RULESET.ruleset_name -> OUTPUT.output_component_name
```

#### Project Configuration Example

```yaml
content: |
  INPUT.kafka -> RULESET.security_rules
  RULESET.security_rules -> OUTPUT.elasticsearch
```

#### Complex Data Flow Example

```yaml
content: |
  # Main data flow
  INPUT.kafka -> RULESET.exclude
  RULESET.exclude -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.compliance_check
  RULESET.compliance_check -> OUTPUT.elasticsearch
  
  # Alert flow
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # Log flow
  RULESET.compliance_check -> OUTPUT.print
```

#### Data Flow Rules Description

**Basic Rules**:
- Use `->` arrows to indicate data flow direction
- Component reference format: `type.component_name`
- Supported types: `INPUT`, `RULESET`, `OUTPUT`
- One data flow definition per line
- Support comments (starting with `#`)

**Data Flow Characteristics**:
- Data flows in the direction of arrows
- One component can have multiple downstream components
- Support branching and merging
- Exclude rulesets are usually placed at the front

**Real Project Example**:

```yaml
content: |
  # Network Security Monitoring Project
  # Data flows from Kafka, goes through multi-layer rule processing, and finally outputs to different targets
  
  INPUT.security_kafka -> RULESET.exclude
  RULESET.exclude -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.behavior_analysis
  RULESET.behavior_analysis -> OUTPUT.security_es
  
  # High-threat events alert separately
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # Debug information printing
  RULESET.behavior_analysis -> OUTPUT.debug_print
```

## 🔧 Part 2: Basic Operating Instructions

### 2.1 Temporary and Official Files

When you click + (New Component) or double-click a component (Edit Existing Component), it will enter the editing mode of the component, you need to pay attention to the fact that when you save in the editing mode (click Save or use Cmd+S shortcut), it will not be directly saved as an official component, it will be saved as a temporary file, and if you need to make it a component that can be used, you need to do Apply after configuration review in Setting -> Push Changes. If you want to become a real component that can be used, you need to review the configuration in Setting -> Push Changes and then Apply.

The HUB will automatically restart the affected projects after the changes are committed.

![PushChanges](png/PushChanges.png)


### 2.2 Reading Configuration from Local Files

Component configurations can also be placed directly into the Config folder of the HUB. After placing the configurations, you need to perform a configuration review in Setting -> Load Local Components and then perform a Load.

The HUB will automatically restart the affected projects after the changes are committed.

![LoadLocalComponents](png/LoadLocalComponents.png)


### 2.3 Flexible Use of Tests and Viewing Sample Data

Output, Ruleset, Plugin, and Project all support testing. For Project testing, you can select Input data input to display the data that needs to be output through Output (it will not really flow into Output component), and Cmd+D is the test shortcut key to quickly wake up the test.
![PluginTest.png](png/PluginTest.png)
![RulesetTest.png](png/RulesetTest.png)
![ProjectTest.png](png/ProjectTest.png)

Each running component will collect Sample Data, we can select “View Sample Data” through the component menu or right-click on the component in the Project flow chart to view the Sample Data. Sample Data is sampled every 6 minutes, and a total of 100 pieces of data are saved.
![SampleData](png/SampleData.png)


### 2.4 Other Features

* All component editing supports smart completions and hints;
  ![EditRuleset.png](png/EditRuleset.png)
* All components support Verify syntax when editing, on the left side of Save button; Input and Output components support Connect Check;
  ![ConnectCheck.png](png/ConnectCheck.png)
* Search box supports not only searching configuration name, but also searching specific configurations within the configuration;
  ![Search.png](png/Search.png)
* Setting supports checking the error reports of HUB and Pluin in Error Logs; Setting's Operations History supports checking the history of configuration commits, project operations, and internal commands issued by the cluster.
  ![Errors.png](png/Errors.png)
  ![OperationsHistory.png](png/OperationsHistory.png)


### 2.5 MCP

AgentSmith-HUB supports MCP, with tokens shared on the server. The following is the Cline configuration:

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

Currently, MCP covers most use cases, including policy editing, etc.
![MCP.png](png/MCP.png)

### 2.6 Authentication and Login (OIDC SSO)

AgentSmith-HUB supports two authentication methods:

- Legacy Token: send `token: <your-token>` in request headers (kept for compatibility);
- OIDC (OpenID Connect): the browser completes login and uses Bearer ID Token; the backend verifies it.

The backend exposes `GET /auth/config` so the frontend can load OIDC settings at runtime. When OIDC is enabled, the login page shows a “Use Single Sign-On” button. The default callback route is `/oidc/callback`.

#### Backend configuration (config.yaml)

```yaml
oidc_enabled: true
oidc_issuer: "https://your-idp/realms/your-realm"   # Issuer for your IdP
oidc_client_id: "agentsmith-web"                    # Client ID registered at IdP
oidc_redirect_uri: "https://hub.example.com/oidc/callback"  # Must be allowed in IdP
oidc_username_claim: "preferred_username"           # Optional; default prefers preferred_username, else email
oidc_allowed_users: ["alice@example.com", "bob"]    # Optional allowlist; empty means nobody will allow
oidc_scope: "openid profile email"                   # Optional; default openid profile email
```

You can also override via environment variables (higher priority):

```bash
OIDC_ENABLED=true
OIDC_ISSUER=https://your-idp/realms/your-realm
OIDC_CLIENT_ID=agentsmith-web
OIDC_REDIRECT_URI=https://hub.example.com/oidc/callback
OIDC_USERNAME_CLAIM=preferred_username
OIDC_ALLOWED_USERS=alice@example.com,bob
OIDC_SCOPE="openid profile email"
```

Notes:

- When `oidc_enabled: true`, you must set `oidc_issuer`, `oidc_client_id`, and `oidc_redirect_uri`.
- `oidc_redirect_uri` must exactly match the IdP client configuration. If Hub is behind a reverse proxy or served under a subpath, set the full callback URL accordingly (e.g., `https://hub.example.com/subpath/oidc/callback`) and allow it at the IdP.
- Username resolution prefers `preferred_username`, then falls back to `email`; override with `oidc_username_claim` if needed.
- If `oidc_allowed_users` is set, only listed users can access; leave empty to deny anyone.
- Legacy Token remains supported for MCP and automation via the `token` header.

Frontend:

The frontend loads OIDC configuration from `GET /auth/config` by default, so no static values are required.

Callback route: `/oidc/callback`


## 📚 Part 3: RULESET Syntax Detailed Explanation

### 3.1 Your First Rule

Assuming we have such incoming data:
```json
{
  "event_type": "login",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "timestamp": 1699999999
}
```

Simplest rule: Detect admin login
```xml
<root author="beginner">
    <rule id="detect_admin_login" name="Detect Admin Login">
        <!-- Independent check, no need for checklist wrapper -->
        <check type="EQU" field="username">admin</check>
        
        <!-- Add marker -->
        <append field="alert">admin login detected</append>
    </rule>
</root>
```

#### 🔍 Syntax Details: `<check>` Tag

`<check>` is the most basic checking unit in the rules engine, used for conditional judgment of data.

**Basic Syntax:**
```xml
<check type="check_type" field="field_name">comparison_value</check>
```

**Attribute Description:**
- `type` (required): Specifies the check type, such as `EQU` (equal), `INCL` (contains), `REGEX` (regex match), etc.
- `field` (required): The data field path to check
- Tag content: Value used for comparison

**Working Principle:**
- The rules engine extracts the field value specified by `field` from the input data
- Uses the comparison method specified by `type` to compare the field value with the tag content
- Returns a check result of true or false

#### 🔍 Syntax Details: `<append>` Tag

`<append>` is used to add new fields to data or modify existing fields.

**Basic Syntax:**
```xml
<append field="field_name">value_to_add</append>
```

**Attribute Description:**
- `field` (required): The field name to add or modify
- `type` (optional): When the value is "PLUGIN", it indicates using a plugin to generate the value

**Working Principle:**
When a rule matches successfully, the `<append>` operation executes, adding the specified field and value to the data.

The output data will become:
```json
{
  "event_type": "login",
  "username": "admin", 
  "source_ip": "192.168.1.100",
  "timestamp": 1699999999,
  "alert": "admin login detected"  // Newly added field
}
```

### 3.2 Adding More Check Conditions

Input data:
```json
{
  "event_type": "login",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "login_time": 23,  // 23:00 (11 PM)
  "failed_attempts": 5
}
```

Detect admin login at unusual time:
```xml
<root author="learner">
    <rule id="suspicious_admin_login" name="Suspicious Admin Login">
        <!-- Flexible order: check username first -->
        <check type="EQU" field="username">admin</check>
        
        <!-- Then check time (late night) -->
        <check type="MT" field="login_time">22</check>  <!-- Greater than 22:00 -->
        
        <!-- Or check failed attempts -->
        <check type="MT" field="failed_attempts">3</check>
        
        <!-- All checks default to AND relationship, all must be satisfied to continue -->
        
        <!-- Add alert information -->
        <append field="risk_level">high</append>
        <append field="alert_reason">admin login at unusual time</append>
        
        <!-- Trigger alert plugin (assuming already configured) -->
        <plugin>send_security_alert(_$ORIDATA)</plugin>
    </rule>
</root>
```

#### 💡 Important Concept: Default Logic for Multiple Condition Checks

When there are multiple `<check>` tags in a rule:
- Default uses **AND** logic: All checks must pass for the rule to match
- Checks execute in order: If a check fails, subsequent checks won't execute (short-circuit evaluation)
- This design improves performance: Fail early, avoid unnecessary checks

In the above example, all three check conditions must be **fully satisfied**:
- username equals "admin"
- login_time greater than 22 (after 10 PM)
- failed_attempts greater than 3

#### 🔍 Syntax Details: `<plugin>` Tag

`<plugin>` is used to execute custom operations, usually for response actions.

**Basic Syntax:**
```xml
<plugin>plugin_name(parameter1, parameter2, ...)</plugin>
```

**Characteristics:**
- Executes operations but doesn't return values to data
- Usually used for external actions: send alerts, execute blocking, record logs, etc.
- Only executes when rule matches successfully

**Difference from `<append type="PLUGIN">`:**
- `<plugin>`: Execute operation, don't return value
- `<append type="PLUGIN">`: Execute plugin and add return value to data

### 3.3 Using Dynamic Values

Input data:
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

Detect transactions exceeding user limit:
```xml
<root author="dynamic_learner">
    <rule id="over_limit_transaction" name="Over Limit Transaction Detection">
        <!-- Dynamic comparison: transaction amount > user daily limit -->
        <check type="MT" field="amount">_$user.daily_limit</check>
        
        <!-- Use plugin to calculate over ratio (assuming custom plugin) -->
        <append type="PLUGIN" field="over_ratio">
            calculate_ratio(amount, user.daily_limit)
        </append>
        
        <!-- Add different processing based on VIP level -->
        <check type="EQU" field="user.vip_level">gold</check>
        <append field="action">notify_vip_service</append>
    </rule>
</root>
```

#### 🔍 Syntax Details: Dynamic Reference (_$ prefix)

The `_$` prefix is used to dynamically reference other field values in the data, rather than using static strings.

**Syntax Format:**
- `_$field_name`: Reference a single field (no need to follow this syntax when used inside plugins).
- `_$parent_field.child_field`: Reference nested fields (no need to follow this syntax when used inside plugins).
- `_$ORIDATA`: Reference the entire original data object (must follow this syntax even when used inside plugins).

**Working Principle:**
- When the rules engine encounters the `_$` prefix, it recognizes it as a dynamic reference; but when applying detection data within plugins, you don't need to use this prefix, just use the field directly.
- Extract the corresponding field value from the currently processed data
- Use the extracted value for comparison or processing

**In the above example:**
- In check, `_$user.daily_limit` extracts the value of `user.daily_limit` from the data (5000);
- In plugin, `amount` extracts the value of the `amount` field (10000); `user.daily_limit` extracts the value of `user.daily_limit` from the data (5000);
- Dynamic comparison: 10000 > 5000, condition satisfied

**Common Usage:**
```xml
<!-- Dynamic comparison of two fields -->
<check type="NEQ" field="current_user">login_user</check>

<!-- Use dynamic values in append -->
<append field="username">_$username</append>

<!-- Use in plugin parameters -->
<plugin>blockIP(malicious_ip, block_duration)</plugin>
```

**Usage of _$ORIDATA:**
`_$ORIDATA` represents the entire original data object, commonly used for:
- Passing complete data to plugins for complex processing
- Generating alerts containing all information
- Data backup or archiving

```xml
<!-- Send entire data object to analysis plugin -->
<append type="PLUGIN" field="risk_analysis">analyzeFullData(_$ORIDATA)</append>

<!-- Generate complete alert -->
<plugin>sendAlert(_$ORIDATA, "HIGH_RISK")</plugin>
```

## 📊 Part 4: Advanced Data Processing

### 4.1 Flexible Execution Order

One of the major features of the rules engine is flexible execution order:

```xml
<rule id="flexible_way" name="Flexible Processing Example">
    <!-- Can add timestamp first -->
    <append type="PLUGIN" field="check_time">now()</append>
    
    <!-- Then perform checks -->
    <check type="EQU" field="event_type">security_event</check>
    
    <!-- Statistical thresholds can be placed anywhere -->
   <threshold group_by="source_ip" range="5m">10</threshold>
    
    <!-- Continue other checks (assuming custom plugins) -->
    <check type="PLUGIN">is_working_hours(check_time)</check>
    
    <!-- Final processing -->
    <append field="processed">true</append>
</rule>
```

#### 💡 Important Concept: Significance of Execution Order

**Why is execution order important?**

- **Data Enhancement**: Can add fields first, then perform checks based on new fields
- **Performance Optimization**: Put fast checks in front, complex operations in back
- **Conditional Processing**: Some operations may depend on results from previous operations

**Execution Flow:**
- The rules engine executes operations according to the appearance order of tags in XML
- If check operations (check, threshold) fail, the rule ends immediately
- Processing operations (append, del, plugin) only execute after all checks pass

#### 🔍 Syntax Details: `<threshold>` Tag

`<threshold>` is used to detect the frequency of events occurring within a specified time window.

**Basic Syntax:**
```xml
<threshold group_by="grouping_field" range="time_range">threshold value</threshold>
```

**Attribute Description:**
- `group_by` (required): Which field to group statistics by, can use multiple fields separated by commas
- `range` (required): Time window, supports s(seconds), m(minutes), h(hours), d(days)
- `value` (required): Trigger threshold, when this quantity is reached the check passes

**Working Principle:**
- Group events by the `group_by` field (e.g., group by source_ip)
- Count events for each group within the sliding time window specified by `range`
- When the statistical value for a group reaches `value`, that threshold check passes

**In the above example:**
- Group by source_ip
- Count events within 5 minutes
- If an IP triggers 10 times within 5 minutes, the threshold check passes

### 4.2 Complex Nested Data Processing

Input data:
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

Rule for processing nested data:
```xml
<root type="DETECTION" author="advanced">
    <rule id="complex_transaction_check" name="Complex Transaction Check">
        <!-- Check basic conditions -->
        <check type="EQU" field="request.method">POST</check>
        <check type="INCL" field="request.url">transfer</check>
        
        <!-- Check amount -->
        <check type="MT" field="request.body.amount">10000</check>
        
        <!-- Add geographic location marker -->
        <append field="geo_risk">_$request.body.metadata.geo.country</append>
        
        <!-- Threshold detection based on geographic location -->
        <threshold group_by="request.body.from_account,request.body.metadata.geo.country" 
                   range="1h">3</threshold>
        
        <!-- Use plugin for deep analysis (assuming custom plugin) -->
        <check type="PLUGIN">analyze_transfer_risk(request.body)</check>
        
        <!-- Extract and process user-agent -->
        <append type="PLUGIN" field="client_info">parseUA(request.headers.user-agent)</append>
        
        <!-- Clean sensitive information -->
        <del>request.headers.authorization</del>
    </rule>
</root>
```

#### 🔍 Syntax Details: `<del>` Tag

`<del>` is used to delete specified fields from data.

**Basic Syntax:**
```xml
<del>field1,field2,field3</del>
```

**Characteristics:**
- Use commas to separate multiple fields
- Support nested field paths: `user.password,session.token`
- If field doesn't exist, won't error, silently ignore
- Only executes when rule matches successfully

**Use Cases:**
- Delete sensitive information (passwords, tokens, keys, etc.)
- Clean temporary fields
- Reduce data volume, avoid transmitting unnecessary information

**In the above example:**
- `request.headers.authorization` contains sensitive authentication information
- Use `<del>` to delete this field after data processing
- Ensure sensitive information won't be stored or transmitted

#### 🔍 Syntax Details: `<iterator>` Tag

`<iterator>` is used to execute a set of checks against each element in an array/list, supporting two evaluation types: `ANY` (pass if any element matches) and `ALL` (all elements must match).

**Basic Syntax:**
```xml
<iterator type="ANY|ALL" field="array_field_path" variable="iteration_variable">
    <!-- May contain: check / threshold / checklist -->
    <check ...>...</check>
    <threshold ...>...</threshold>
    <checklist condition="...">
        <check id="..." ...>...</check>
        <!-- May also contain threshold -->
    </checklist>
</iterator>
```

**Attributes:**
- `type` (required): `ANY` or `ALL`
- `field` (required): The array field path to iterate; supports:
  - Native arrays: `[]interface{}`, `[]string`, `[]map[string]interface{}`
  - JSON string whose content is an array (auto-parsed)
- `variable` (required): Iteration variable name for accessing the current element
  - Naming rules: start with a letter or underscore; only letters, digits, underscores
  - Must not conflict with internal prefixes or reserved names (e.g., `_$`, `ORIDATA`)

**Execution Semantics:**
- Iteration context: during `<iterator>`, the default context is replaced with a new context that contains only the iteration variable: `{variable: current_element}`.
  - Within inner `<check>`/`<threshold>`/`<checklist>` nodes, access the current element via the iteration variable, e.g., `proc.name`, `_ip`, `item.value`.
- `ANY`: If any element makes the inner checks pass as a whole, iterator returns true.
- `ALL`: Only if every element makes the inner checks pass, iterator returns true.

**Example 1: String array (any one is a public IP)**
```json
{"ips": ["1.2.3.4", "10.0.0.1", "8.8.8.8"]}
```
```xml
<rule id="ip_any_public" name="Any Public IP">
    <iterator type="ANY" field="ips" variable="_ip">
        <check type="PLUGIN">!isPrivateIP(_ip)</check>
    </iterator>
</rule>
```

**Example 2: Object array (all processes are not browsers)**
```json
{"processes": [{"name":"powershell.exe","cmd":"..."},{"name":"svchost.exe","cmd":"..."}]}
```
```xml
<rule id="no_browser" name="Process Whitelist">
    <iterator type="ALL" field="processes" variable="proc">
        <check type="NI" field="proc.name" logic="AND" delimiter="|">chrome.exe|firefox.exe</check>
        <checklist condition="a or b">
            <check id="a" type="INCL" field="proc.name">powershell</check>
            <check id="b" type="INCL" field="proc.cmd">-EncodedCommand</check>
        </checklist>
    </iterator>
</rule>
```

**Example 3: JSON string array (all end with .com)**
```json
{"targets": "[\"example.com\", \"test.com\"]"}
```
```xml
<rule id="domains_all_com" name="Domain Suffix Check">
    <iterator type="ALL" field="targets" variable="_domain">
        <check type="END" field="_domain">.com</check>
    </iterator>
</rule>
```

### 4.3 Conditional Combination Logic (checklist)

Input data:
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

Rule using conditional combinations:
```xml
<root author="logic_master">
    <rule id="malware_detection" name="Malware Detection">
        <!-- Method 1: Use independent checks (default AND relationship) -->
        <check type="END" field="filename">.exe</check>
        <check type="MT" field="size">1000000</check>  <!-- Greater than 1MB -->
        
        <!-- Method 2: Use checklist for complex logic combinations -->
        <checklist condition="suspicious_file and (email_threat or unknown_hash)">
            <check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
                .exe|.dll|.scr|.bat
            </check>
            <check id="email_threat" type="INCL" field="sender">suspicious.com</check>
            <check id="unknown_hash" type="PLUGIN">
                is_known_malware(hash)
            </check>
        </checklist>
        
        <!-- Enrich data -->
        <append type="PLUGIN" field="virus_scan">virusTotal(hash)</append>
        <append field="threat_level">high</append>
        
        <!-- Automatic response (assuming custom plugin) -->
        <plugin>quarantine_file(filename)</plugin>
        <plugin>notify_security_team(_$ORIDATA)</plugin>
    </rule>
</root>
```

#### 🔍 Syntax Details: `<checklist>` Tag

`<checklist>` allows you to use custom logical expressions to combine multiple check conditions.

**Basic Syntax:**
```xml
<checklist condition="logical_expression">
    <check id="identifier1" ...>...</check>
    <check id="identifier2" ...>...</check>
</checklist>
```

**Attribute Description:**
- `condition` (required): Logical expression built using the `id` of check nodes

**Logical Expression Syntax:**
- Use `and`, `or` to connect conditions
- Use `()` for grouping, controlling precedence
- Use `not` for negation
- Only use lowercase logical operators

**Example Expressions:**
- `a and b and c`: All conditions satisfied
- `a or b or c`: Any condition satisfied
- `(a or b) and not c`: a or b satisfied, and c not satisfied
- `a and (b or (c and d))`: Complex nested conditions

**Example of using threshold in checklist:**
```xml
<checklist condition="suspicious_activity and high_frequency">
    <check id="suspicious_activity" type="INCL" field="command">powershell|cmd|wmic</check>
    <threshold id="high_frequency" group_by="source_ip" range="5m">10</threshold>
</checklist>
```
- Check if command contains suspicious keywords
- Also check if source IP triggers more than 10 times within 5 minutes
- Checklist passes when both conditions are satisfied

**Working Principle:**
- Execute all check nodes and threshold nodes with `id`, record the result of each node (true/false)
- Substitute results into the `condition` expression to calculate final result
- If final result is true, checklist passes

**Supported Node Types:**
- `<check>` nodes: Execute field checks, regex matching, plugin calls, etc.
- `<threshold>` nodes: Execute threshold detection, supporting counting, summing, classification statistics, etc.

#### 🔍 Syntax Details: Multi-value Matching (logic and delimiter)

When you need to check if a field matches multiple values, you can use multi-value matching syntax.

**Basic Syntax:**
```xml
<check type="type" field="field" logic="OR|AND" delimiter="separator">
    value1separatorvalue2separatorvalue3
</check>
```

**Attribute Description:**
- `logic`: "OR" or "AND", specifies the logical relationship between multiple values
- `delimiter`: Separator, used to split multiple values

**Working Principle:**
- Use `delimiter` to split tag content into multiple values
- Check each value separately
- Determine final result based on `logic`:
   - `logic="OR"`: Any value matches returns true
   - `logic="AND"`: All values must match to return true

**In the above example:**
```xml
<check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
    .exe|.dll|.scr|.bat
</check>
```
- Check if filename contains .exe, .dll, .scr, or .bat
- Use OR logic: Any extension matches is sufficient
- Use | as separator

## 🔧 Part 5: Advanced Features Detailed Explanation

### 5.1 Three Modes of Threshold Detection

The `<threshold>` tag can not only perform simple counting, but also supports three powerful statistical modes:

- **Default Mode (Counting)**: Count event occurrences
- **SUM Mode**: Sum specified fields
- **CLASSIFY Mode**: Count different values (deduplication counting)

#### Scenario 1: Login Failure Count Statistics (Default Counting)

Input data stream:
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

Rule:
```xml
<rule id="brute_force_detection" name="Brute Force Detection">
    <check type="EQU" field="event">login_failed</check>
    
    <!-- 5 failures for same user and IP within 5 minutes -->
   <threshold group_by="user,ip" range="5m">5</threshold>
    
    <append field="alert_type">brute_force_attempt</append>
    <plugin>block_ip(ip, 3600)</plugin>  <!-- Block for 1 hour -->
</rule>
```

#### Scenario 2: Transaction Amount Statistics (SUM Mode)

Input data stream:
```json
// Today's transactions
{"event": "transfer", "user": "alice", "amount": 5000}
{"event": "transfer", "user": "alice", "amount": 8000}
{"event": "transfer", "user": "alice", "amount": 40000}  // Total 53000, triggered!
```

Rule:
```xml
<rule id="daily_limit_check" name="Daily Limit Check">
    <check type="EQU" field="event">transfer</check>
    
    <!-- Cumulative amount exceeds 50000 within 24 hours -->
    <threshold group_by="user" range="24h" count_type="SUM" 
               count_field="amount">50000</threshold>
    
    <append field="action">freeze_account</append>
</rule>
```

#### 🔍 Advanced Syntax: SUM Mode of threshold

**Attribute Description:**
- `count_type="SUM"`: Enable summation mode
- `count_field` (required): Field name to sum
- `value`: Trigger when cumulative sum reaches this value

**Working Principle:**
- Group by `group_by`
- Accumulate values of `count_field` within time window
- Trigger when cumulative value reaches `value`

#### Scenario 3: Resource Access Statistics (CLASSIFY Mode)

Input data stream:
```json
{"user": "bob", "action": "download", "file_id": "doc001"}
{"user": "bob", "action": "download", "file_id": "doc002"}
{"user": "bob", "action": "download", "file_id": "doc003"}
// ... accessed 26 different files
```

Rule:
```xml
<rule id="data_exfiltration_check" name="Data Exfiltration Check">
    <check type="EQU" field="action">download</check>
    
    <!-- Access more than 25 different files within 1 hour -->
    <threshold group_by="user" range="1h" count_type="CLASSIFY" 
               count_field="file_id">25</threshold>
    
    <append field="risk_score">high</append>
    <plugin>alert_dlp_team(_$ORIDATA)</plugin>
</rule>
```

#### 🔍 Advanced Syntax: CLASSIFY Mode of threshold

**Attribute Description:**
- `count_type="CLASSIFY"`: Enable deduplication counting mode
- `count_field` (required): Field to count different values
- `value`: Trigger when number of different values reaches this value

**Working Principle:**
- Group by `group_by`
- Collect all different values of `count_field` within time window
- Trigger when number of different values reaches `value`

**Use Cases:**
- Detect scanning behavior (access multiple different ports/IPs)
- Data exfiltration detection (access multiple different files)
- Anomaly behavior detection (use multiple different accounts)

### 5.2 Built-in Plugin System

AgentSmith-HUB provides rich built-in plugins that can be used without additional development.

#### 🧩 Complete List of Built-in Plugins

##### Check Type Plugins (Return bool)

| Plugin Name | Function | Parameters | Example |
|-------------|----------|------------|---------|
| `isPrivateIP` | Check if IP is private address | ip (string) | `isPrivateIP(source_ip)` |
| `cidrMatch` | Check if IP is in CIDR range | ip (string), cidr (string) | `cidrMatch(client_ip, "192.168.1.0/24")` |
| `geoMatch` | Check IP's country | ip (string), countryISO (string) | `geoMatch(source_ip, "US")` |
| `suppressOnce` | Alert suppression | key (any), windowSec (int), ruleid (string, optional) | `suppressOnce(alert_key, 300, "rule_001")` |

##### Data Processing Plugins (Return various types)

#### Time Processing Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `now` | Get current timestamp | Optional: format (unix/ms/rfc3339) | `now()` |
| `dayOfWeek` | Get day of week (0-6, 0=Sunday) | Optional: timestamp (int64) | `dayOfWeek()` |
| `hourOfDay` | Get hour (0-23) | Optional: timestamp (int64) | `hourOfDay()` |
| `tsToDate` | Convert timestamp to RFC3339 format | timestamp (int64) | `tsToDate(timestamp)` |

#### Encoding and Hash Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `base64Encode` | Base64 encoding | input (string) | `base64Encode(data)` |
| `base64Decode` | Base64 decoding | input (string) | `base64Decode(encoded_data)` |
| `hashMD5` | MD5 hash | input (string) | `hashMD5(data)` |
| `hashSHA1` | SHA1 hash | input (string) | `hashSHA1(data)` |
| `hashSHA256` | SHA256 hash | input (string) | `hashSHA256(data)` |

#### URL Parsing Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `extractDomain` | Extract domain from URL | urlOrHost (string) | `extractDomain(url)` |
| `extractTLD` | Extract top-level domain | domain (string) | `extractTLD(domain)` |
| `extractSubdomain` | Extract subdomain | host (string) | `extractSubdomain(host)` |

#### String Processing Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `replace` | String replacement | input (string), old (string), new (string) | `replace(text, "old", "new")` |
| `regexExtract` | Regex extraction | input (string), pattern (string) | `regexExtract(text, "\\d+")` |
| `regexReplace` | Regex replacement | input (string), pattern (string), replacement (string) | `regexReplace(text, "\\d+", "NUMBER")` |

#### Data Parsing Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `parseJSON` | Parse JSON string | jsonString (string) | `parseJSON(json_data)` |
| `parseUA` | Parse User-Agent | userAgent (string) | `parseUA(user_agent)` |

#### Threat Intelligence Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `virusTotal` | VirusTotal query | hash (string), apiKey (string, optional) | `virusTotal(file_hash)` |
| `shodan` | Shodan query | ip (string), apiKey (string, optional) | `shodan(ip_address)` |
| `threatBook` | ThreatBook query | queryValue (string), queryType (string), apiKey (string, optional) | `threatBook(ip, "ip")` |

**Note on plugin parameter format**:
- When referencing fields in data, no need to use `_$` prefix, just use field name directly: `source_ip`
- When completely referencing all original data: `_$ORIDATA`
- When using static values, use strings directly (with quotes): `"192.168.1.0/24"`
- When using numbers, no quotes needed: `300`

## Part 6: Ruleset Best Practices

### 6.1 Complex Logic Combinations

```xml
<rule id="complex_plugin_usage" name="Complex Plugin Usage">
    <!-- Use checklist to combine multiple conditions -->
    <checklist condition="(private_ip or suspicious_country) and not excluded">
  <check id="private_ip" type="PLUGIN">isPrivateIP(source_ip)</check>
  <check id="suspicious_country" type="PLUGIN">geoMatch(source_ip, "CN")</check>
  <check id="excluded" type="PLUGIN">cidrMatch(source_ip, "10.0.0.0/8")</check>
</checklist>
    
    <!-- Data enrichment -->
    <append type="PLUGIN" field="threat_intel">virusTotal(file_hash)</append>
    <append type="PLUGIN" field="geo_info">shodan(source_ip)</append>
    
    <!-- Time-related processing -->
    <append type="PLUGIN" field="hour">hourOfDay()</append>
    <check type="PLUGIN">hourOfDay() > 22</check>
</rule>
```

### 6.2 Alert Suppression Example

```xml
<rule id="suppression_example" name="Alert Suppression">
    <check type="EQU" field="event_type">login_failed</check>
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_brute_force")</check>
    <append field="alert_type">brute_force</append>
</rule>
```

### 6.3 Data Transformation Example

```xml
<rule id="data_transformation" name="Data Transformation">
    <check type="EQU" field="content_type">json</check>
    
    <!-- Parse JSON and extract fields -->
    <append type="PLUGIN" field="parsed_data">parseJSON(raw_content)</append>
    <append field="user_id">parsed_data.user.id</append>
    
    <!-- Encoding processing -->
    <append type="PLUGIN" field="encoded">base64Encode(sensitive_data)</append>
    
    <!-- Hash calculation -->
    <append type="PLUGIN" field="content_hash">hashSHA256(raw_content)</append>
</rule>
```

#### Built-in Plugin Usage Examples

##### Network Security Scenario

Input data:
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

Rule using built-in plugins:
```xml
<rule id="suspicious_connection" name="Suspicious Connection Detection">
    <!-- Check if it's external connection -->
    <check type="PLUGIN">isPrivateIP(source_ip)</check>  <!-- Source is internal -->
    <check type="PLUGIN">!isPrivateIP(dest_ip)</check>  <!-- Target is external -->
    
    <!-- Check geographic location -->
    <append type="PLUGIN" field="dest_country">geoMatch(dest_ip)</append>
    
    <!-- Add timestamp -->
    <append type="PLUGIN" field="detection_time">now()</append>
    <append type="PLUGIN" field="detection_hour">hourOfDay()</append>
    
    <!-- Calculate data exfiltration risk -->
    <check type="MT" field="bytes_sent">1000000</check>  <!-- Greater than 1MB -->
    
    <!-- Generate alert -->
    <append field="alert_type">potential_data_exfiltration</append>
    
    <!-- Query threat intelligence (if configured) -->
    <append type="PLUGIN" field="threat_intel">threatBook(dest_ip, "ip")</append>
</rule>
```

##### Threat Intelligence Detection Scenario

Demonstrating the advantages of flexible execution order: check basic conditions first, then query threat intelligence, finally make decisions based on results.

Input data:
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

Threat intelligence detection rule:
```xml
<rule id="threat_intel_detection" name="Threat Intelligence Detection">
    <!-- Step 1: Check data type, quick filtering -->
    <check type="EQU" field="datatype">external_connection</check>
   
    <!-- Step 2: Confirm target IP is public address -->
    <check type="PLUGIN">!isPrivateIP(dest_ip)</check>

    <!-- Step 3: Query threat intelligence, enrich data -->
    <append type="PLUGIN" field="threat_intel">threatBook(dest_ip, "ip")</append>
    
    <!-- Step 4: Parse threat intelligence results -->
    <append type="PLUGIN" field="threat_level">
        parseJSON(threat_intel)
    </append>
    
    <!-- Step 5: Make judgments based on threat level -->
    <checklist condition="high_threat or (medium_threat and has_data_transfer)">
        <check id="high_threat" type="EQU" field="threat_level">high</check>
        <check id="medium_threat" type="EQU" field="threat_level">medium</check>
        <check id="has_data_transfer" type="MT" field="bytes_sent">1000</check>
    </checklist>
    
    <!-- Step 6: Enrich alert information -->
    <append field="alert_title">Malicious IP Communication Detected</append>
    <append type="PLUGIN" field="ip_reputation">
        parseJSON(threat_intel.reputation_score)
    </append>
    <append type="PLUGIN" field="threat_tags">
        parseJSON(threat_intel.tags)
    </append>
    
    <!-- Step 7: Generate detailed alert (assuming custom plugin) -->
    <plugin>generateThreatAlert(_$ORIDATA, threat_intel)</plugin>
</rule>
```

#### 💡 Key Advantages Demonstration

This example demonstrates several key advantages of flexible execution order:

1. **Performance Optimization**: Execute fast checks first (datatype), avoid querying threat intelligence for all data
2. **Progressive Enhancement**: Confirm it's a public IP first, then query threat intelligence, avoid invalid queries
3. **Dynamic Decision Making**: Dynamically adjust subsequent processing based on threat intelligence return results
4. **Conditional Response**: Only execute response operations for high threat levels
5. **Data Utilization**: Fully utilize rich data returned by threat intelligence

If using fixed execution order, you cannot implement this flexible processing method of "query intelligence first, then make decisions based on results".

##### Log Analysis Scenario

Input data:
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

Log processing rule:
```xml
<rule id="log_analysis" name="Error Log Analysis">
    <check type="EQU" field="log_level">ERROR</check>
    
    <!-- Parse JSON data -->
    <append type="PLUGIN" field="parsed_body">parseJSON(request_body)</append>
    
    <!-- Parse User-Agent -->
    <append type="PLUGIN" field="browser_info">parseUA(user_agent)</append>
    
    <!-- Extract error information -->
    <append type="PLUGIN" field="error_type">
        regexExtract(stack_trace, "([A-Za-z.]+Exception)")
    </append>
    
    <!-- Time processing -->
    <append type="PLUGIN" field="readable_time">tsToDate(timestamp)</append>
    <append type="PLUGIN" field="hour">hourOfDay(timestamp)</append>
    
    <!-- Data masking -->
    <append type="PLUGIN" field="sanitized_message">
        regexReplace(message, "password\":\"[^\"]+", "password\":\"***")
    </append>
    
    <!-- Alert suppression: same type error only alert once in 5 minutes -->
    <check type="PLUGIN">suppressOnce(error_type, 300, "error_log_analysis")</check>
    
    <!-- Generate alert (assuming custom plugin) -->
    <plugin>sendToElasticsearch(_$ORIDATA)</plugin>
</rule>
```

##### Data Masking and Security Processing

```xml
<rule id="data_masking" name="Data Masking Processing">
    <check type="EQU" field="contains_sensitive_data">true</check>
    
    <!-- Data hashing -->
    <append type="PLUGIN" field="user_id_hash">hashSHA256(user_id)</append>
    <append type="PLUGIN" field="session_hash">hashMD5(session_id)</append>
    
    <!-- Sensitive information encoding -->
    <append type="PLUGIN" field="encoded_payload">base64Encode(sensitive_payload)</append>
    
    <!-- Clean and replace -->
    <append type="PLUGIN" field="cleaned_log">replace(raw_log, user_password, "***")</append>
    <append type="PLUGIN" field="masked_phone">regexReplace(phone_number, "(\\d{3})\\d{4}(\\d{4})", "$1****$2")</append>
    
    <!-- Delete original sensitive data -->
    <del>user_password,raw_sensitive_data,unencrypted_payload</del>
</rule>
```

#### ⚠️ Alert Suppression Best Practices (suppressOnce)

The alert suppression plugin can prevent the same alert from triggering repeatedly in a short time.

**Why do we need the ruleid parameter?**

If you don't use the `ruleid` parameter, suppression of the same key by different rules will affect each other:

```xml
<!-- Rule A: Network threat detection -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(source_ip, 300)</check>
</rule>

<!-- Rule B: Login anomaly detection -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(source_ip, 300)</check>
</rule>
```

**Problem**: After Rule A triggers, Rule B will also be suppressed for the same IP!

**Correct Usage**: Use `ruleid` parameter to isolate different rules:

```xml
<!-- Rule A: Network threat detection -->
<rule id="network_threat">
    <check type="PLUGIN">suppressOnce(source_ip, 300, "network_threat")</check>
</rule>

<!-- Rule B: Login anomaly detection -->  
<rule id="login_anomaly">
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_anomaly")</check>
</rule>
```

### 6.4 Exclude Ruleset

Exclude is used to filter out data that doesn't need processing (ruleset type is EXCLUDE). Special behavior of exclude:
- When exclude rule matches, data is "not allowed to pass" (i.e., filtered out, no longer continue processing, data will be discarded)
- When all exclude rules don't match, data continues to be passed to subsequent processing

```xml
<root type="EXCLUDE" name="security_exclude" author="security_team">
    <!-- Exclude rule 1: Trusted IPs -->
    <rule id="trusted_ips">
        <check type="INCL" field="source_ip" logic="OR" delimiter="|">
            10.0.0.1|10.0.0.2|10.0.0.3
        </check>
        <append field="excluded">true</append>
    </rule>
    
    <!-- Exclude rule 2: Known benign processes -->
    <rule id="benign_processes">
        <check type="INCL" field="process_name" logic="OR" delimiter="|">
            chrome.exe|firefox.exe|explorer.exe
        </check>
        <!-- Can add multiple check conditions, all must be satisfied to be exclude filtered -->
        <check type="PLUGIN">isPrivateIP(source_ip)</check>
    </rule>
    
    <!-- Exclude rule 3: Internal test traffic -->
    <rule id="test_traffic">
        <check type="INCL" field="user_agent">internal-testing-bot</check>
        <check type="PLUGIN">cidrMatch(source_ip, "192.168.100.0/24")</check>
    </rule>
</root>
```

## 🚨 Part 7: Real-World Case Studies

### 7.1 Case Study 1: APT Attack Detection

Complete APT attack detection ruleset (using built-in plugins and hypothetical custom plugins):

```xml
<root type="DETECTION" name="apt_detection_suite" author="threat_hunting_team">
    <!-- Rule 1: PowerShell Empire Detection -->
    <rule id="powershell_empire" name="PowerShell Empire C2 Detection">
        <!-- Flexible order: enrichment before detection -->
        <append type="PLUGIN" field="decoded_cmd">base64Decode(command_line)</append>
        
        <!-- Check process name -->
        <check type="INCL" field="process_name">powershell</check>
        
        <!-- Detect Empire characteristics -->
        <check type="INCL" field="decoded_cmd" logic="OR" delimiter="|">
            System.Net.WebClient|DownloadString|IEX|Invoke-Expression
        </check>
        
        <!-- Detect encoded commands -->
        <check type="INCL" field="command_line">-EncodedCommand</check>
        
        <!-- Network connection detection -->
        <threshold group_by="hostname" range="10m">3</threshold>
        
        <!-- Threat intelligence query -->
        <append type="PLUGIN" field="c2_url">
            regexExtract(decoded_cmd, "https?://[^\\s]+")
        </append>
        
        <!-- Generate IoC -->
        <append field="ioc_type">powershell_empire_c2</append>
        <append type="PLUGIN" field="ioc_hash">hashSHA256(decoded_cmd)</append>
        
        <!-- Automatic response (hypothetical custom plugin) -->
        <plugin>isolateHost(hostname)</plugin>
        <plugin>extractAndShareIoCs(_$ORIDATA)</plugin>
    </rule>
    
    <!-- Rule 2: Lateral Movement Detection -->
    <rule id="lateral_movement" name="Lateral Movement Detection">
        <!-- Multiple lateral movement technique detection -->
        <checklist condition="(wmi_exec or psexec or rdp_brute) and not internal_scan">
            <!-- WMI execution -->
            <check id="wmi_exec" type="INCL" field="process_name">wmic.exe</check>
            <!-- PsExec -->
            <check id="psexec" type="INCL" field="service_name">PSEXESVC</check>
            <!-- RDP brute force -->
            <check id="rdp_brute" type="EQU" field="event_id">4625</check>
            <!-- Exclude internal scanning -->
            <check id="internal_scan" type="PLUGIN">
                isPrivateIP(source_ip)
            </check>
        </checklist>
        
        <!-- Time window detection -->
        <threshold group_by="source_ip,dest_ip" range="30m">5</threshold>
        
        <!-- Risk scoring (hypothetical custom plugin) -->
        <append type="PLUGIN" field="risk_score">
            calculateLateralMovementRisk(ORIDATA)
        </append>
        
        <plugin>updateAttackGraph(source_ip, dest_ip)</plugin>
    </rule>
    
    <!-- Rule 3: Data Exfiltration Detection -->
    <rule id="data_exfiltration" name="Data Exfiltration Detection">
        <!-- First check if accessing sensitive data -->
        <check type="INCL" field="file_path" logic="OR" delimiter="|">
            /etc/passwd|/etc/shadow|.ssh/|.aws/credentials
        </check>

        <!-- Check outbound behavior -->
        <check type="PLUGIN">!isPrivateIP(dest_ip)</check>
        
        <!-- Abnormal transfer detection -->
        <threshold group_by="source_ip" range="1h" count_type="SUM" 
                   count_field="bytes_sent">1073741824</threshold>  <!-- 1GB -->
        
        <!-- DNS tunnel detection (hypothetical custom plugin) -->
        <checklist condition="dns_tunnel_check">
            <check id="dns_tunnel_check" type="PLUGIN">
                detectDNSTunnel(dns_queries)
            </check>
        </checklist>
        
        <!-- Generate alert -->
        <append field="alert_severity">critical</append>
        <append type="PLUGIN" field="data_classification">
            classifyData(file_path)
        </append>
        
        <plugin>blockDataTransfer(source_ip, dest_ip)</plugin>
        <plugin>triggerIncidentResponse(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 7.2 Case Study 2: Real-Time Financial Fraud Detection

```xml
<root type="DETECTION" name="fraud_detection_system" author="risk_team">
    <!-- Rule 1: Account Takeover Detection -->
    <rule id="account_takeover" name="Account Takeover Detection">
        <!-- Real-time device fingerprinting (hypothetical custom plugin) -->
        <append type="PLUGIN" field="device_fingerprint">
            generateFingerprint(user_agent, screen_resolution, timezone)
        </append>
        
        <!-- Check device changes (hypothetical custom plugin) -->
        <check type="PLUGIN">
            isNewDevice(user_id, device_fingerprint)
        </check>
        
        <!-- Geographic location anomaly (hypothetical custom plugin) -->
        <append type="PLUGIN" field="geo_distance">
            calculateGeoDistance(user_id, current_ip, last_ip)
        </append>
        <check type="MT" field="geo_distance">500</check>  <!-- 500km -->
        
        <!-- Behavioral pattern analysis (hypothetical custom plugin) -->
        <append type="PLUGIN" field="behavior_score">
            analyzeBehaviorPattern(user_id, recent_actions)
        </append>
        
        <!-- Transaction speed detection -->
        <threshold group_by="user_id" range="10m">5</threshold>
        
        <!-- Risk decision (hypothetical custom plugin) -->
        <append type="PLUGIN" field="risk_decision">
            makeRiskDecision(behavior_score, geo_distance, device_fingerprint)
        </append>
        
        <!-- Real-time intervention (hypothetical custom plugin) -->
        <plugin>requireMFA(user_id, transaction_id)</plugin>
        <plugin>notifyUser(user_id, "suspicious_activity")</plugin>
    </rule>
    
    <!-- Rule 2: Money Laundering Detection -->
    <rule id="money_laundering" name="Money Laundering Detection">
        <!-- Smurfing-layering-integration pattern (hypothetical custom plugin) -->
        <checklist condition="structuring or layering or integration">
            <!-- Structuring -->
            <check id="structuring" type="PLUGIN">
                detectStructuring(user_id, transaction_history)
            </check>
            <!-- Layering -->
            <check id="layering" type="PLUGIN">
                detectLayering(account_network, transaction_flow)
            </check>
            <!-- Integration -->
            <check id="integration" type="PLUGIN">
                detectIntegration(merchant_category, transaction_pattern)
            </check>
        </checklist>
        
        <!-- Correlation analysis (hypothetical custom plugin) -->
        <append type="PLUGIN" field="network_risk">
            analyzeAccountNetwork(user_id, connected_accounts)
        </append>
        
        <!-- Cumulative amount monitoring -->
        <threshold group_by="account_cluster" range="7d" count_type="SUM"
                   count_field="amount">1000000</threshold>
        
        <!-- Compliance reporting (hypothetical custom plugin) -->
        <append type="PLUGIN" field="sar_report">
            generateSAR(_$ORIDATA)  <!-- Suspicious Activity Report -->
        </append>
        
        <plugin>freezeAccountCluster(account_cluster)</plugin>
        <plugin>notifyCompliance(sar_report)</plugin>
    </rule>
</root>
```

### 7.3 Case Study 3: Zero Trust Security Architecture

```xml
<root type="DETECTION" name="zero_trust_security" author="security_architect">
    <!-- Rule 1: Continuous Authentication -->
    <rule id="continuous_auth" name="Continuous Authentication">
        <!-- Verify on every request -->
        <check type="NOTNULL" field="auth_token"></check>
        
        <!-- Validate token (hypothetical custom plugin) -->
        <check type="PLUGIN">validateToken(auth_token)</check>
        
        <!-- Context awareness (hypothetical custom plugin) -->
        <append type="PLUGIN" field="trust_score">
            calculateTrustScore(
                user_id,
                device_trust,
                network_location,
                behavior_baseline,
                time_of_access
            )
        </append>
        
        <!-- Dynamic permission adjustment -->
        <checklist condition="low_trust or anomaly_detected">
            <check id="low_trust" type="LT" field="trust_score">0.7</check>
            <check id="anomaly_detected" type="PLUGIN">
                detectAnomaly(current_behavior, baseline_behavior)
            </check>
        </checklist>
        
        <!-- Micro-segmentation policy (hypothetical custom plugin) -->
        <append type="PLUGIN" field="allowed_resources">
            applyMicroSegmentation(trust_score, requested_resource)
        </append>
        
        <!-- Real-time policy enforcement (hypothetical custom plugin) -->
        <plugin>enforcePolicy(user_id, allowed_resources)</plugin>
        <plugin>logZeroTrustDecision(_$ORIDATA)</plugin>
    </rule>
    
    <!-- Rule 2: Device Trust Assessment -->
    <rule id="device_trust" name="Device Trust Assessment">
        <!-- Device health check (hypothetical custom plugin) -->
        <append type="PLUGIN" field="device_health">
            checkDeviceHealth(device_id)
        </append>
        
        <!-- Compliance validation (hypothetical custom plugin) -->
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
        
        <!-- Certificate validation (hypothetical custom plugin) -->
        <check type="PLUGIN">
            validateDeviceCertificate(device_cert)
        </check>
        
        <!-- Trust scoring (hypothetical custom plugin) -->
        <append type="PLUGIN" field="device_trust_score">
            calculateDeviceTrust(_$ORIDATA)
        </append>
        
        <!-- Access decision (hypothetical custom plugin) -->
        <plugin>applyDevicePolicy(device_id, device_trust_score)</plugin>
    </rule>
</root>
```

## 📖 Part 8: Syntax Reference Manual

### 8.1 Ruleset Structure

#### Root Element `<root>`
```xml
<root type="DETECTION|EXCLUDE" name="ruleset_name" author="author">
    <!-- Rule list -->
</root>
```

| Attribute | Required | Description | Default |
|-----------|----------|-------------|---------|
| type | No | Ruleset type, DETECTION type passes through after match, EXCLUDE doesn't pass through after match | DETECTION |
| name | No | Ruleset name | - |
| author | No | Author information | - |

#### Rule Element `<rule>`
```xml
<rule id="unique_identifier" name="rule_description">
    <!-- Operation list: execute in order of appearance -->
</rule>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| id | Yes | Unique rule identifier |
| name | No | Human-readable rule description |

#### Multiple Rules Relationship

When a ruleset contains multiple `<rule>` elements, they have an **OR relationship**:

**Core Concept:**
- **Independent Evaluation**: Each rule is evaluated independently against the input data
- **OR Logic**: If ANY rule matches, a data record is generated and passed downstream
- **Multiple Matches**: Multiple rules can match the same data, generating multiple records
- **No Sequential Dependencies**: Rules do not depend on each other's processing results

**Execution Flow:**
1. **Parallel Evaluation**: All rules in the ruleset are evaluated against the same input data
2. **Match Detection**: Each rule that matches generates a separate data record
3. **Data Generation**: Each matched rule creates its own output with its specific enrichments
4. **Downstream Flow**: All generated records are passed to the next component in the data flow

**Example:**
```xml
<root type="DETECTION" name="multi_rule_example">
    <!-- Rule 1: Detects admin logins -->
    <rule id="admin_login">
        <check type="EQU" field="username">admin</check>
        <append field="alert_type">admin_login</append>
        <append field="severity">high</append>
    </rule>
    
    <!-- Rule 2: Detects failed logins -->
    <rule id="failed_login">
        <check type="EQU" field="result">failure</check>
        <append field="alert_type">failed_login</append>
        <append field="severity">medium</append>
    </rule>
    
    <!-- Rule 3: Detects unusual time access -->
    <rule id="unusual_time">
        <check type="MT" field="hour">22</check>
        <append field="alert_type">unusual_time</append>
        <append field="severity">low</append>
    </rule>
</root>
```

**Input Data:**
```json
{"username": "admin", "result": "success", "hour": 23}
```

**Output:**
- Rule 1 matches → generates: `{"username": "admin", "result": "success", "hour": 23, "alert_type": "admin_login", "severity": "high"}`
- Rule 3 matches → generates: `{"username": "admin", "result": "success", "hour": 23, "alert_type": "unusual_time", "severity": "low"}`

**Key Points:**
- **Independent Processing**: Each rule processes the original input data independently
- **Multiple Outputs**: One input can generate multiple output records
- **No Data Sharing**: Rules cannot share data modifications with each other
- **Performance**: All rules are evaluated, so rule order doesn't affect performance

### 8.2 Check Operations

#### Independent Check `<check>`
```xml
<check type="type" field="field_name" logic="OR|AND" delimiter="separator">
    value
</check>
```

| Attribute | Required | Description | Applicable Scenarios |
|-----------|----------|-------------|---------------------|
| type | Yes | Check type | All |
| field | Conditional | Field name (optional for PLUGIN type) | Required for non-PLUGIN types |
| logic | No | Multi-value logic | When using delimiter |
| delimiter | Conditional | Value separator | Required when using logic |
| id | Conditional | Node identifier | Required when using condition in checklist |

#### Check List `<checklist>`
```xml
<checklist condition="logical_expression">
    <check id="a" ...>...</check>
    <check id="b" ...>...</check>
</checklist>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| condition | No | Logical expression (e.g., `a and (b or c)`) |

### 8.3 Complete List of Check Types

#### String Matching Types
| Type | Description | Case Sensitive | Example |
|------|-------------|----------------|---------|
| EQU | Exact equality | Insensitive | `<check type="EQU" field="status">active</check>` |
| NEQ | Exact inequality | Insensitive | `<check type="NEQ" field="status">inactive</check>` |
| INCL | Contains substring | Sensitive | `<check type="INCL" field="message">error</check>` |
| NI | Doesn't contain substring | Sensitive | `<check type="NI" field="message">success</check>` |
| START | Starts with | Sensitive | `<check type="START" field="path">/admin</check>` |
| END | Ends with | Sensitive | `<check type="END" field="file">.exe</check>` |
| NSTART | Doesn't start with | Sensitive | `<check type="NSTART" field="path">/public</check>` |
| NEND | Doesn't end with | Sensitive | `<check type="NEND" field="file">.txt</check>` |

#### Case-Insensitive Types
| Type | Description | Example |
|------|-------------|---------|
| NCS_EQU | Case-insensitive equality | `<check type="NCS_EQU" field="protocol">HTTP</check>` |
| NCS_NEQ | Case-insensitive inequality | `<check type="NCS_NEQ" field="method">get</check>` |
| NCS_INCL | Case-insensitive contains | `<check type="NCS_INCL" field="header">Content-Type</check>` |
| NCS_NI | Case-insensitive doesn't contain | `<check type="NCS_NI" field="useragent">bot</check>` |
| NCS_START | Case-insensitive starts with | `<check type="NCS_START" field="domain">WWW.</check>` |
| NCS_END | Case-insensitive ends with | `<check type="NCS_END" field="email">.COM</check>` |
| NCS_NSTART | Case-insensitive doesn't start with | `<check type="NCS_NSTART" field="url">HTTP://</check>` |
| NCS_NEND | Case-insensitive doesn't end with | `<check type="NCS_NEND" field="filename">.EXE</check>` |

#### Numeric Comparison Types
| Type | Description | Example |
|------|-------------|---------|
| MT | Greater than | `<check type="MT" field="score">80</check>` |
| LT | Less than | `<check type="LT" field="age">18</check>` |

#### Null Value Check Types
| Type | Description | Example |
|------|-------------|---------|
| ISNULL | Field is null | `<check type="ISNULL" field="optional_field"></check>` |
| NOTNULL | Field is not null | `<check type="NOTNULL" field="required_field"></check>` |

#### Advanced Matching Types
| Type | Description | Example |
|------|-------------|---------|
| REGEX | Regular expression | `<check type="REGEX" field="ip">^\d+\.\d+\.\d+\.\d+$</check>` |
| PLUGIN | Plugin function (supports `!` negation) | `<check type="PLUGIN">isValidEmail(email)</check>` |

### 8.4 Frequency Detection

#### Threshold Detection `<threshold>`
```xml
<threshold group_by="field1,field2" range="time_range"
           count_type="SUM|CLASSIFY" count_field="statistical_field" local_cache="true|false">threshold value</threshold>
```

| Attribute | Required | Description | Example |
|-----------|----------|-------------|---------|
| group_by | Yes | Grouping fields | `source_ip,user_id` |
| range | Yes | Time range | `5m`, `1h`, `24h` |
| value | Yes | Threshold | `10` |
| count_type | No | Count type | Default: count, `SUM`: sum, `CLASSIFY`: deduplication count |
| count_field | Conditional | Statistical field | Required when using SUM/CLASSIFY |
| local_cache | No | Use local cache | `true` or `false` |

### 8.5 Data Processing Operations

#### Field Append `<append>`
```xml
<append field="field_name" type="PLUGIN">value or plugin call</append>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| field | Yes | Field name to add |
| type | No | Append type (`PLUGIN` indicates plugin call) |

#### Field Delete `<del>`
```xml
<del>field1,field2,field3</del>
```

#### Plugin Execution `<plugin>`
```xml
<plugin>plugin_function(parameter1, parameter2)</plugin>
```

### 8.6 Field Access Syntax

#### Basic Access
- **Direct field**: `field_name`
- **Nested field**: `parent.child.grandchild`
- **Array index**: `array.#0.field` (access first element)

#### Dynamic Reference (_$ prefix)
- **Field reference**: `_$field_name`
- **Nested reference**: `_$parent.child.field`
- **Original data**: `_$ORIDATA`
- **Array index**: `_$array.#0.field` (access first element)

#### Example Comparison
```xml
<!-- Static value -->
<check type="EQU" field="status">active</check>

<!-- Dynamic value -->
<check type="EQU" field="status">_$expected_status</check>

<!-- Nested field -->
<check type="EQU" field="user.profile.role">admin</check>

<!-- Dynamic nested -->
<check type="EQU" field="current_level">_$config.min_level</check>
```

### 8.7 Performance Optimization Recommendations

#### Operation Order Optimization
```xml
<!-- Recommended: High-performance operations first -->
<rule id="optimized">
    <check type="NOTNULL" field="required"></check>     <!-- Fastest -->
    <check type="EQU" field="type">target</check>       <!-- Fast -->
    <check type="INCL" field="message">keyword</check>  <!-- Medium -->
    <check type="REGEX" field="data">pattern</check>    <!-- Slow -->
    <check type="PLUGIN">complex_check()</check>        <!-- Slowest -->
</rule>
```

#### Threshold Configuration Optimization
```xml
<!-- Use local cache to improve performance -->
<threshold group_by="user_id" range="5m" local_cache="true">10</threshold>

<!-- Avoid overly large time windows -->
<threshold group_by="ip" range="1h">1000</threshold>  <!-- Don't exceed 24h -->
```

### 8.8 Common Errors and Solutions

#### XML Syntax Errors
```xml
<!-- Error: Special characters not escaped -->
<check type="INCL" field="xml"><tag>value</tag></check>

<!-- Correct: Use CDATA -->
<check type="INCL" field="xml"><![CDATA[<tag>value</tag>]]></check>
```

#### Logic Errors
```xml
<!-- Error: Reference non-existent id in condition -->
<checklist condition="a and b">
    <check type="EQU" field="status">active</check>  <!-- Missing id -->
</checklist>

<!-- Correct -->
<checklist condition="a and b">
    <check id="a" type="EQU" field="status">active</check>
    <check id="b" type="NOTNULL" field="user"></check>
</checklist>
```

#### Performance Issues
```xml
<!-- Problem: Directly use plugin on large amounts of data -->
<rule id="slow">
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>

<!-- Optimization: Filter first, then process -->
<rule id="fast">
    <check type="EQU" field="type">target</check>
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>
```

### 8.9 Debugging Tips

#### 1. Use append to track execution flow
```xml
<rule id="debug_flow">
    <append field="_debug_step1">check started</append>
    <check type="EQU" field="type">target</check>
    
    <append field="_debug_step2">check passed</append>
   <threshold group_by="user" range="5m">10</threshold>
    
    <append field="_debug_step3">threshold passed</append>
    <!-- Final data will contain all debug fields, showing execution flow -->
</rule>
```

### 8.10 Iterator `<iterator>`

#### Basic Syntax
```xml
<iterator type="ANY|ALL" field="array_field_path" variable="iteration_variable">
    <!-- Inner nodes can include: check / threshold / checklist -->
    ...
</iterator>
```

#### Attributes
| Attribute | Required | Description |
|-----------|----------|-------------|
| type | Yes | Evaluation: `ANY` passes if any element matches; `ALL` passes only if all elements match |
| field | Yes | Array field path to iterate; supports native arrays or JSON string arrays |
| variable | Yes | Iteration variable name; must start with letter/underscore, contain only letters/digits/underscores; must not conflict with internal prefixes or reserved names (e.g., `_$`, `ORIDATA`) |

#### Iteration Context and Field Access
- Within the iterator body, the default context is replaced with `{variable: current_element}`.
- In child nodes (`<check>`/`<threshold>`/`<checklist>`), access the current element via the iteration variable:
  - Object element: `proc.name`, `item.value`
  - Scalar element (e.g., string): use the variable directly: `_ip`

#### Supported Data Types
- `[]interface{}`, `[]string`, `[]map[string]interface{}`
- String whose content is a JSON array (auto-parsed)

#### Result Evaluation
- `ANY`: Return true if any element passes the inner checks as a whole
- `ALL`: Return true only if all elements pass; otherwise false

#### Example
```xml
<iterator type="ANY" field="ips" variable="_ip">
    <check type="PLUGIN">!isPrivateIP(_ip)</check>
</iterator>
```

#### 2. Test single rule
Create a ruleset containing only the rule to be tested:
```xml
<root type="DETECTION" name="test_single_rule">
    <rule id="test_rule">
        <!-- Your test rule -->
    </rule>
</root>
```

#### 3. Verify field access
Use append to verify if fields are correctly obtained:
```xml
<rule id="verify_fields">
    <append field="debug_nested">_$user.profile.settings.theme</append>
    <append field="debug_array">_$items.0.name</append>
    <!-- Check debug field values in output -->
</rule>
```

## Part 9: Custom Plugin Development

### 9.1 Plugin Classification

#### By Runtime Method
- **Local Plugin**: Built-in plugins compiled into the program, highest performance
- **Yaegi Plugin**: Dynamic plugins running with Yaegi interpreter, **supports stateful and init functions**

#### By Return Type
- **Check Node Plugin**: returns `(bool, error)` for use in `<check type="PLUGIN">`, `<append type="PLUGIN">` and `<plugin>`.
- **Other Plugin**: returns `(interface{}, bool, error)`, used in `<append type="PLUGIN">` and `<plugin>`, if the second parameter returns false, then the Append action will not be executed.

### 9.2 Plugin Syntax

#### Basic Syntax
```xml
<!-- Check type plugin -->
<check type="PLUGIN">pluginName(param1, param2, ...)</check>

<!-- Data processing plugin -->
<append type="PLUGIN" field="field_name">pluginName(param1, param2, ...)</append>

<!-- Execute operation plugin -->
<plugin>pluginName(param1, param2, ...)</plugin>
```

#### Parameter Types
- **String**: `"value"` or `'value'`
- **Number**: `123` or `123.45`
- **Boolean**: `true` or `false`
- **Field reference**: `field_name` or `parent.child.field`
- **Original data**: `_$ORIDATA` (only one that needs _$ prefix)

#### Negation Syntax
Check type plugins support negation prefix:
```xml
<check type="PLUGIN">!isPrivateIP(source_ip)</check>
```

### 9.3 Plugin Function Signatures

#### Check Type Plugin
```go
package plugin

import (
    "errors"
    "fmt"
)

// Eval function must return (bool, error)
func Eval(args ...interface{}) (bool, error) {
    if len(args) == 0 {
        return false, errors.New("plugin requires at least one argument")
    }
    
    // Parameter processing
    data := args[0]
    
    // Plugin logic
    if someCondition {
        return true, nil
    }
    
    return false, nil
}
```

#### Data Processing Plugin
```go
package plugin

import (
    "errors"
    "fmt"
)

// Eval function must return (interface{}, bool, error)
func Eval(args ...interface{}) (interface{}, bool, error) {
    if len(args) == 0 {
        return nil, false, errors.New("plugin requires at least one argument")
    }
    
    // Parameter processing
    input := args[0]
    
    // Data processing logic
    result := processData(input)
    
    return result, true, nil
}
```

### 9.4 Stateful Features of Yaegi Plugins

#### State Maintenance Mechanism
```go
// Yaegi plugins support global variables and init functions
var (
    cache = make(map[string]interface{})
    cacheMutex sync.RWMutex
    lastUpdate time.Time
)

// init function executes when plugin loads
func init() {
    // Initialize cache
    refreshCache()
}

// Stateful Eval function
func Eval(key string) (interface{}, bool, error) {
    cacheMutex.RLock()
    if value, exists := cache[key]; exists {
        cacheMutex.RUnlock()
        return value, true, nil
    }
    cacheMutex.RUnlock()
    
    // Calculate and cache result
    result := computeResult(key)
    cacheMutex.Lock()
    cache[key] = result
    cacheMutex.Unlock()
    
    return result, true, nil
}
```

### 9.5 Plugin Limitations
- Only the Go standard library can be used, no third-party packages;
- A function named `Eval` must be defined, and the package must be a plugin;
- The function return value must strictly match the requirements.


## Summary

Remember the core philosophy: **Combine as needed, arrange flexibly**. Based on your specific requirements, freely combine various operations to create the most suitable rules.

Happy using! 🚀