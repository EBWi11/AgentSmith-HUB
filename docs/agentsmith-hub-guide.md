# 🛡️ AgentSmith-HUB Guide

The AgentSmith-HUB rules engine is a powerful real-time data processing engine that can:
- 🔍 **Real-time Detection**: Identify threats and anomalies from data streams
- 🔄 **Data Transformation**: Process and enrich data
- 📊 **Statistical Analysis**: Perform threshold detection and frequency analysis
- 🚨 **Automatic Response**: Trigger alerts and automated operations

### Core Philosophy: Flexible Execution Order

The rules engine adopts a **flexible execution order**, where operations are executed according to their appearance order in XML, allowing you to freely combine various operations based on specific requirements.

## 📋 Part One: Core Component Syntax

### 1.1 INPUT Syntax

INPUT defines data input sources, supporting multiple data source types.

#### Basic Syntax
```yaml
type: "data_source_type"
# specific configuration parameters
```

#### Supported Data Source Types

##### Kafka Input
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
    - "localhost:9093"
  topic: "security_events"
  group: "agentsmith_consumer"
  compression: "snappy"  # optional: none, snappy, gzip
  # SASL authentication (optional)
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS configuration (optional)
  tls:
    enable: true
    ca_file: "/path/to/ca.pem"
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

##### Aliyun SLS Input
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
  query: "* | where attack_type_name != 'null'"  # optional query filter
```

##### Kafka Azure Input
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

##### Kafka AWS Input
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

### 1.2 OUTPUT Syntax

OUTPUT defines the output destination for data processing results.

#### Basic Syntax
```yaml
type: "output_type"
# specific configuration parameters
```

#### Supported Output Types

##### Print Output (Console Print)
```yaml
type: print
```

##### Kafka Output
```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
    - "localhost:9093"
  topic: "processed_events"
  key: "user_id"  # optional: specify message key field
  compression: "snappy"  # optional: none, snappy, gzip
  # SASL authentication (optional)
  sasl:
    enable: true
    mechanism: "plain"
    username: "your_username"
    password: "your_password"
  # TLS configuration (optional)
  tls:
    enable: true
    ca_file: "/path/to/ca.pem"
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

##### Elasticsearch Output
```yaml
type: elasticsearch
elasticsearch:
  hosts:
    - "http://localhost:9200"
    - "https://localhost:9201"
  index: "security-events-{YYYY.MM.DD}"  # supports time patterns
  batch_size: 1000  # batch write size
  flush_dur: "5s"   # flush interval
  # authentication configuration (optional)
  auth:
    type: basic  # basic, api_key, bearer
    username: "elastic"
    password: "password"
    # or use API Key
    # api_key: "your-api-key"
    # or use Bearer Token
    # token: "your-bearer-token"
```

### 1.3 PROJECT Syntax

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
  INPUT.kafka -> RULESET.whitelist
  RULESET.whitelist -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.compliance_check
  RULESET.compliance_check -> OUTPUT.elasticsearch
  
  # Alert flow
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # Log flow
  RULESET.compliance_check -> OUTPUT.print
```

#### Data Flow Rules

**Basic Rules:**
- Use `->` arrow to indicate data flow direction
- Component reference format: `type.component_name`
- Supported types: `INPUT`, `RULESET`, `OUTPUT`
- One data flow definition per line
- Support comments (starting with `#`)

**Data Flow Characteristics:**
- Data flows in the direction of arrows
- One component can have multiple downstream components
- Support branching and merging
- Whitelist rulesets are usually placed first

**Real Project Example:**

```yaml
content: |
  # Network security monitoring project
  # Data flows from Kafka, processed through multiple rule layers, finally output to different targets
  
  INPUT.security_kafka -> RULESET.whitelist
  RULESET.whitelist -> RULESET.threat_detection
  RULESET.threat_detection -> RULESET.behavior_analysis
  RULESET.behavior_analysis -> OUTPUT.security_es
  
  # High threat events alert separately
  RULESET.threat_detection -> OUTPUT.alert_kafka
  
  # Debug information print
  RULESET.behavior_analysis -> OUTPUT.debug_print
```

## 📚 Part Two: RULESET Syntax Details

### 2.1 Your First Rule

Suppose we have such data flowing in:
```json
{
  "event_type": "login",
  "username": "admin",
  "source_ip": "192.168.1.100",
  "timestamp": 1699999999
}
```

The simplest rule: detect admin login
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
1. The rules engine extracts the field value specified by `field` from the input data
2. Uses the comparison method specified by `type` to compare the field value with the tag content
3. Returns a check result of true or false

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

### 2.2 Adding More Check Conditions

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

Detect admin login at unusual times:
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
        
        <!-- Trigger alert plugin (assuming configured) -->
        <plugin>send_security_alert(_$ORIDATA)</plugin>
    </rule>
</root>
```

#### 💡 Important Concept: Default Logic for Multiple Condition Checks

When there are multiple `<check>` tags in a rule:
- Default uses **AND** logic: All checks must pass for the rule to match
- Checks execute in order: If a check fails, subsequent checks won't execute (short-circuit evaluation)
- This design improves performance: fail early, avoid unnecessary checks

In the above example, all three check conditions must be **fully satisfied**:
1. username equals "admin" 
2. login_time greater than 22 (after 10 PM)
3. failed_attempts greater than 3

#### 🔍 Syntax Details: `<plugin>` Tag

`<plugin>` is used to execute custom operations, typically for response actions.

**Basic Syntax:**
```xml
<plugin>plugin_name(parameter1, parameter2, ...)</plugin>
```

**Characteristics:**
- Executes operations but doesn't return values to data
- Typically used for external actions: sending alerts, executing blocks, logging, etc.
- Only executes when rule matches successfully

**Difference from `<append type="PLUGIN">`:**
- `<plugin>`: Executes operation, doesn't return value
- `<append type="PLUGIN">`: Executes plugin and adds return value to data

### 2.3 Using Dynamic Values

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

Detect transactions exceeding user limits:
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

The `_$` prefix is used to dynamically reference other field values in data, rather than using static strings.

**Syntax Format:**
- `_$field_name`: Reference single field (no need to follow this syntax when used inside plugins).
- `_$parent_field.child_field`: Reference nested field (no need to follow this syntax when used inside plugins).
- `_$ORIDATA`: Reference the entire original data object (need to follow this syntax even when used inside plugins).

**Working Principle:**
1. When the rules engine encounters the `_$` prefix, it recognizes it as a dynamic reference; but when applying detection data inside plugins, this prefix is not needed, just use the field directly.
2. Extract the corresponding field value from the currently processed data
3. Use the extracted value for comparison or processing

**In the above example:**
- In check, `_$user.daily_limit` extracts the value of `user.daily_limit` from data (5000);
- In plugin, `amount` extracts the value of `amount` field (10000); `user.daily_limit` extracts the value of `user.daily_limit` from data (5000);
- Dynamic comparison: 10000 > 5000, condition satisfied

**Common Usage:**
```xml
<!-- Dynamic comparison of two fields -->
<check type="NEQ" field="current_user">login_user</check>

<!-- Use dynamic value in append -->
<append field="username">_$username</append>

<!-- Use in plugin parameters -->
<plugin>blockIP(malicious_ip, block_duration)</plugin>
```

**_$ORIDATA Usage:**
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

## 📊 Part Three: Advanced Data Processing

### 3.1 Flexible Execution Order

One of the major features of the rules engine is flexible execution order:

```xml
<rule id="flexible_way" name="Flexible Processing Example">
    <!-- Can add timestamp first -->
    <append type="PLUGIN" field="check_time">now()</append>
    
    <!-- Then perform checks -->
    <check type="EQU" field="event_type">security_event</check>
    
    <!-- Statistical thresholds can be placed anywhere -->
    <threshold group_by="source_ip" range="5m" value="10"/>
    
    <!-- Continue with other checks (assuming custom plugins) -->
    <check type="PLUGIN">is_working_hours(check_time)</check>
    
    <!-- Final processing -->
    <append field="processed">true</append>
</rule>
```

#### 💡 Important Concept: Significance of Execution Order

**Why is execution order important?**

1. **Data Enhancement**: Can add fields first, then perform checks based on new fields
2. **Performance Optimization**: Place fast checks first, complex operations later
3. **Conditional Processing**: Some operations may depend on results from previous operations

**Execution Flow:**
1. The rules engine executes operations according to the appearance order of tags in XML
2. If check operations (check, threshold) fail, the rule ends immediately
3. Processing operations (append, del, plugin) only execute after all checks pass

#### 🔍 Syntax Details: `<threshold>` Tag

`<threshold>` is used to detect the frequency of events occurring within a specified time window.

**Basic Syntax:**
```xml
<threshold group_by="grouping_field" range="time_range" value="threshold"/>
```

**Attribute Description:**
- `group_by` (required): Which field to group statistics by, can use multiple fields separated by commas
- `range` (required): Time window, supports s(seconds), m(minutes), h(hours), d(days)
- `value` (required): Trigger threshold, when this quantity is reached, the check passes

**Working Principle:**
1. Group events by the `group_by` field (e.g., by source_ip)
2. Count events for each group within the sliding time window specified by `range`
3. When a group's statistical value reaches `value`, that check passes

**In the above example:**
- Group by source_ip
- Count events within 5 minutes
- If an IP triggers 10 times within 5 minutes, the threshold check passes

### 3.2 Complex Nested Data Processing

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

Rules for processing nested data:
```xml
<root type="DETECTION" author="advanced">
    <rule id="complex_transaction_check" name="Complex Transaction Detection">
        <!-- Check basic conditions -->
        <check type="EQU" field="request.method">POST</check>
        <check type="INCL" field="request.url">transfer</check>
        
        <!-- Check amount -->
        <check type="MT" field="request.body.amount">10000</check>
        
        <!-- Add geographic location marker -->
        <append field="geo_risk">_$request.body.metadata.geo.country</append>
        
        <!-- Threshold detection based on geographic location -->
        <threshold group_by="request.body.from_account,request.body.metadata.geo.country" 
                   range="1h" value="3"/>
        
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
- Supports nested field paths: `user.password,session.token`
- If field doesn't exist, won't error, silently ignored
- Only executes when rule matches successfully

**Use Cases:**
- Delete sensitive information (passwords, tokens, keys, etc.)
- Clean temporary fields
- Reduce data volume, avoid transmitting unnecessary information

**In the above example:**
- `request.headers.authorization` contains sensitive authentication information
- Use `<del>` to delete this field after data processing
- Ensure sensitive information won't be stored or transmitted

### 3.3 Conditional Combination Logic

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

Rules using conditional combinations:
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

## 🚨 Part Five: Practical Case Studies

### Case 1: APT Attack Detection

Complete APT attack detection ruleset (using built-in plugins and assumed custom plugins):

```xml
<root type="DETECTION" name="apt_detection_suite" author="threat_hunting_team">
    <!-- Rule 1: PowerShell Empire Detection -->
    <rule id="powershell_empire" name="PowerShell Empire C2 Detection">
        <!-- Flexible order: enrichment first, then detection -->
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
        <threshold group_by="hostname" range="10m" value="3"/>
        
        <!-- Threat intelligence query -->
        <append type="PLUGIN" field="c2_url">
            regexExtract(decoded_cmd, "https?://[^\\s]+")
        </append>
        
        <!-- Generate IoC -->
        <append field="ioc_type">powershell_empire_c2</append>
        <append type="PLUGIN" field="ioc_hash">hashSHA256(decoded_cmd)</append>
        
        <!-- Automatic response (assuming custom plugin) -->
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
        <threshold group_by="source_ip,dest_ip" range="30m" value="5"/>
        
        <!-- Risk scoring (assuming custom plugin) -->
        <append type="PLUGIN" field="risk_score">
            calculateLateralMovementRisk(ORIDATA)
        </append>
        
        <plugin>updateAttackGraph(source_ip, dest_ip)</plugin>
    </rule>
    
    <!-- Rule 3: Data Exfiltration Detection -->
    <rule id="data_exfiltration" name="Data Exfiltration Detection">
        <!-- First check if it's sensitive data access -->
        <check type="INCL" field="file_path" logic="OR" delimiter="|">
            /etc/passwd|/etc/shadow|.ssh/|.aws/credentials
        </check>

        <!-- Check external connection behavior -->
        <check type="PLUGIN">!isPrivateIP(dest_ip)</check>
       
        <!-- Anomalous transmission detection -->
        <threshold group_by="source_ip" range="1h" count_type="SUM" 
                   count_field="bytes_sent" value="1073741824"/>  <!-- 1GB -->
        
        <!-- DNS tunnel detection (assuming custom plugin) -->
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

### Case 2: Real-time Financial Fraud Detection

```xml
<root type="DETECTION" name="fraud_detection_system" author="risk_team">
    <!-- Rule 1: Account Takeover Detection -->
    <rule id="account_takeover" name="Account Takeover Detection">
        <!-- Real-time device fingerprinting (assuming custom plugin) -->
        <append type="PLUGIN" field="device_fingerprint">
            generateFingerprint(user_agent, screen_resolution, timezone)
        </append>
        
        <!-- Check device changes (assuming custom plugin) -->
        <check type="PLUGIN">
            isNewDevice(user_id, device_fingerprint)
        </check>
        
        <!-- Geographic location anomaly (assuming custom plugin) -->
        <append type="PLUGIN" field="geo_distance">
            calculateGeoDistance(user_id, current_ip, last_ip)
        </append>
        <check type="MT" field="geo_distance">500</check>  <!-- 500km -->
        
        <!-- Behavior pattern analysis (assuming custom plugin) -->
        <append type="PLUGIN" field="behavior_score">
            analyzeBehaviorPattern(user_id, recent_actions)
        </append>
        
        <!-- Transaction speed detection -->
        <threshold group_by="user_id" range="10m" value="5"/>
        
        <!-- Risk decision (assuming custom plugin) -->
        <append type="PLUGIN" field="risk_decision">
            makeRiskDecision(behavior_score, geo_distance, device_fingerprint)
        </append>
        
        <!-- Real-time intervention (assuming custom plugin) -->
        <plugin>requireMFA(user_id, transaction_id)</plugin>
        <plugin>notifyUser(user_id, "suspicious_activity")</plugin>
    </rule>
    
    <!-- Rule 2: Money Laundering Behavior Detection -->
    <rule id="money_laundering" name="Money Laundering Behavior Detection">
        <!-- Structuring-Layering-Integration pattern (assuming custom plugin) -->
        <checklist condition="structuring or layering or integration">
            <!-- Structuring -->
            <check id="structuring" type="PLUGIN">
                detectStructuring(user_id, transaction_history)
            </check>
            <!-- Layering -->
            <check id="layering" type="PLUGIN">
                detectLayering(account_network, transaction_flow)
            </check>
            <!-- Integration phase -->
            <check id="integration" type="PLUGIN">
                detectIntegration(merchant_category, transaction_pattern)
            </check>
        </checklist>
        
        <!-- Correlation analysis (assuming custom plugin) -->
        <append type="PLUGIN" field="network_risk">
            analyzeAccountNetwork(user_id, connected_accounts)
        </append>
        
        <!-- Cumulative amount monitoring -->
        <threshold group_by="account_cluster" range="7d" count_type="SUM"
                   count_field="amount" value="1000000"/>
        
        <!-- Compliance reporting (assuming custom plugin) -->
        <append type="PLUGIN" field="sar_report">
            generateSAR(_$ORIDATA)  <!-- Suspicious Activity Report -->
        </append>
        
        <plugin>freezeAccountCluster(account_cluster)</plugin>
        <plugin>notifyCompliance(sar_report)</plugin>
    </rule>
</root>
```

### Case 3: Zero Trust Security Architecture

```xml
<root type="DETECTION" name="zero_trust_security" author="security_architect">
    <!-- Rule 1: Continuous Authentication -->
    <rule id="continuous_auth" name="Continuous Authentication">
        <!-- Verify every request -->
        <check type="NOTNULL" field="auth_token"></check>
        
        <!-- Verify token validity (assuming custom plugin) -->
        <check type="PLUGIN">validateToken(auth_token)</check>
        
        <!-- Context awareness (assuming custom plugin) -->
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
        
        <!-- Micro-segmentation strategy (assuming custom plugin) -->
        <append type="PLUGIN" field="allowed_resources">
            applyMicroSegmentation(trust_score, requested_resource)
        </append>
        
        <!-- Real-time policy enforcement (assuming custom plugin) -->
        <plugin>enforcePolicy(user_id, allowed_resources)</plugin>
        <plugin>logZeroTrustDecision(_$ORIDATA)</plugin>
    </rule>
    
    <!-- Rule 2: Device Trust Assessment -->
    <rule id="device_trust" name="Device Trust Assessment">
        <!-- Device health check (assuming custom plugin) -->
        <append type="PLUGIN" field="device_health">
            checkDeviceHealth(device_id)
        </append>
        
        <!-- Compliance verification (assuming custom plugin) -->
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
        
        <!-- Certificate verification (assuming custom plugin) -->
        <check type="PLUGIN">
            validateDeviceCertificate(device_cert)
        </check>
        
        <!-- Trust scoring (assuming custom plugin) -->
        <append type="PLUGIN" field="device_trust_score">
            calculateDeviceTrust(_$ORIDATA)
        </append>
        
        <!-- Access decision (assuming custom plugin) -->
        <plugin>applyDevicePolicy(device_id, device_trust_score)</plugin>
    </rule>
</root>
```

## 📖 Part Six: Syntax Reference Manual

### 6.1 Ruleset Structure

#### Root Element `<root>`
```xml
<root type="DETECTION|WHITELIST" name="ruleset_name" author="author">
    <!-- rule list -->
</root>
```

| Attribute | Required | Description | Default |
|-----------|----------|-------------|---------|
| type | No | Rule set type, DETECTION type for hits passed backward, WHITELIST for hits not passed backward | DETECTION |
| name | No | Ruleset name | - |
| author | No | Author information | - |

#### Rule Element `<rule>`
```xml
<rule id="unique_identifier" name="rule_description">
    <!-- operation list: execute in appearance order -->
</rule>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| id | Yes | Unique rule identifier |
| name | No | Human-readable rule description |

### 6.2 Check Operations

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

### 6.3 Complete List of Check Types

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

#### Null Check Types
| Type | Description | Example |
|------|-------------|---------|
| ISNULL | Field is null | `<check type="ISNULL" field="optional_field"></check>` |
| NOTNULL | Field is not null | `<check type="NOTNULL" field="required_field"></check>` |

#### Advanced Matching Types
| Type | Description | Example |
|------|-------------|---------|
| REGEX | Regular expression | `<check type="REGEX" field="ip">^\d+\.\d+\.\d+\.\d+$</check>` |
| PLUGIN | Plugin function (supports `!` negation) | `<check type="PLUGIN">isValidEmail(email)</check>` |

### 6.4 Data Processing Operations

#### Threshold Detection `<threshold>`
```xml
<threshold group_by="field1,field2" range="time_range" value="threshold" 
           count_type="SUM|CLASSIFY" count_field="stat_field" local_cache="true|false"/>
```

| Attribute | Required | Description | Example |
|-----------|----------|-------------|---------|
| group_by | Yes | Grouping fields | `source_ip,user_id` |
| range | Yes | Time range | `5m`, `1h`, `24h` |
| value | Yes | Threshold | `10` |
| count_type | No | Count type | Default: count, `SUM`: sum, `CLASSIFY`: deduplication count |
| count_field | Conditional | Statistic field | Required when using SUM/CLASSIFY |
| local_cache | No | Use local cache | `true` or `false` |

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

### 6.5 Field Access Syntax

#### Basic Access
- **Direct field**: `field_name`
- **Nested field**: `parent.child.grandchild`
- **Array index**: `array.0.field` (access first element)

#### Dynamic Reference (_$ prefix)
- **Field reference**: `_$field_name`
- **Nested reference**: `_$parent.child.field`
- **Original data**: `_$ORIDATA`

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

### 6.6 Built-in Plugin Quick Reference

#### Check Plugins (return bool)
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| isPrivateIP | Check private IP | ip | `isPrivateIP(ip)` |
| cidrMatch | CIDR match | ip, cidr | `cidrMatch(ip, "10.0.0.0/8")` |
| geoMatch | Geographic location match | ip, country | `geoMatch(ip, "US")` |
| suppressOnce | Alert suppression | key, seconds, ruleid | `suppressOnce(ip, 300, "rule1")` |

#### Data Processing Plugins (return various types)
| Plugin | Function | Return Type | Example |
|--------|----------|-------------|---------|
| now | Current time | int64 | `now()` |
| base64Encode | Base64 encoding | string | `base64Encode(data)` |
| hashSHA256 | SHA256 hash | string | `hashSHA256(content)` |
| parseJSON | JSON parsing | object | `parseJSON(json_str)` |
| regexExtract | Regex extraction | string | `regexExtract(text, pattern)` |

### 6.7 Performance Optimization Suggestions

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
<threshold group_by="user_id" range="5m" value="10" local_cache="true"/>

<!-- Avoid overly large time windows -->
<threshold group_by="ip" range="1h" value="1000"/>  <!-- Don't exceed 24h -->
```

### 6.8 Common Errors and Solutions

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
<!-- Problem: Use plugins directly on large amounts of data -->
<rule id="slow">
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>

<!-- Optimization: Filter first, then process -->
<rule id="fast">
    <check type="EQU" field="type">target</check>
    <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>
```

### 5.9 Debugging Tips

#### 1. Use append to track execution flow
```xml
<rule id="debug_flow">
    <append field="_debug_step1">check started</append>
    <check type="EQU" field="type">target</check>
    
    <append field="_debug_step2">check passed</append>
    <threshold group_by="user" range="5m" value="10"/>
    
    <append field="_debug_step3">threshold passed</append>
    <!-- Final data will contain all debug fields, showing execution flow -->
</rule>
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

## 5. Custom Plugin Development

### 5.1 Plugin Classification

#### By Runtime Method
- **Local Plugin**: Built-in plugins compiled into the program, highest performance
- **Yaegi Plugin**: Dynamic plugins run using Yaegi interpreter, **supports stateful and init functions**

#### By Return Type
- **Check Plugin**: Returns `(bool, error)`, used in `<check type="PLUGIN">`
- **Data Processing Plugin**: Returns `(interface{}, bool, error)`, used in `<append type="PLUGIN">` and `<plugin>`

### 5.2 Plugin Syntax

#### Basic Syntax
```xml
<!-- Check plugins -->
<check type="PLUGIN">pluginName(param1, param2, ...)</check>

<!-- Data processing plugins -->
<append type="PLUGIN" field="field_name">pluginName(param1, param2, ...)</append>

<!-- Action plugins -->
<plugin>pluginName(param1, param2, ...)</plugin>
```

#### Parameter Types
- **String**: `"value"` or `'value'`
- **Number**: `123` or `123.45`
- **Boolean**: `true` or `false`
- **Field reference**: `field_name` or `parent.child.field`
- **Original data**: `_$ORIDATA` (only one that needs _$ prefix)

#### Negation Syntax
Check plugins support negation prefix:
```xml
<check type="PLUGIN">!isPrivateIP(source_ip)</check>
```

### 5.3 Plugin Function Signatures

#### Check Plugins
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

#### Data Processing Plugins
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

### 5.4 Yaegi Plugin Stateful Features

#### State Persistence Mechanism
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

### 5.5 Plugin Limitations
- Can only use Go standard library
- Cannot use third-party packages
- Must define function named `Eval`
- Function signature must strictly match

### 5.6 Common Standard Libraries
- Basic: `fmt`, `strings`, `strconv`, `errors`
- Encoding: `encoding/json`, `encoding/base64`, `encoding/hex`
- Crypto: `crypto/md5`, `crypto/sha256`, `crypto/rand`
- Time: `time`
- Regex: `regexp`
- Network: `net`, `net/url`
- Concurrency: `sync`

## 6. Best Practices

### 6.1 Performance Optimization
1. **Execution order**: Fast checks first, slow operations later
2. **Cache usage**: Leverage built-in plugin caching mechanisms
3. **Avoid repeated calculations**: Reasonable use of field references

### 6.2 Error Handling
1. **Parameter validation**: Plugins must validate parameters internally
2. **Graceful degradation**: Return reasonable default values on errors
3. **Logging**: Record important operations

### 6.3 Stateful Plugin Design
```go
// Thread-safe state management
var (
    dataCache = make(map[string]interface{})
    cacheMutex sync.RWMutex
    lastRefresh time.Time
    refreshInterval = 10 * time.Minute
)

func init() {
    // Initialize cache
    refreshCache()
    
    // Start background refresh task
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            refreshCache()
        }
    }()
}

func Eval(key string) (interface{}, bool, error) {
    // Check if refresh is needed
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
    // Cache refresh logic
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    
    // Execute refresh operation
    lastRefresh = time.Now()
}
```

### 6.4 Debugging Tips
1. **Use append for tracking**: Add debug fields
2. **Step-by-step testing**: Test plugin functions individually
3. **Verify fields**: Ensure field references are correct

### 6.5 Common Patterns

#### Cache Pattern
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
    
    // Calculate and cache
    result := expensiveCalculation(key)
    mutex.Lock()
    cache[key] = result
    mutex.Unlock()
    
    return result, true, nil
}
```

#### Counter Pattern
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

#### Time Window Pattern
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
            return false, nil // Within time window, don't trigger
        }
    }
    
    lastSeen[key] = now
    return true, nil // Trigger
}
```

## Summary

AgentSmith-HUB's plugin system provides powerful extension capabilities:

1. **Rich types**: Supports check and data processing plugins
2. **State management**: Yaegi plugins support stateful and init functions
3. **Concurrency safety**: Supports thread-safe state management
4. **Performance optimization**: Built-in caching and background task support
5. **Easy development**: Clear function signatures and error handling mechanisms

Through reasonable use of plugins, complex data processing workflows and business logic can be built, achieving flexible security detection and response mechanisms.

## 🔧 Part Seven: Custom Plugin Development

## 🎯 Summary

Core advantages of AgentSmith-HUB rules engine:

1. **Completely flexible execution order**: Operations execute according to appearance order in XML
2. **Concise syntax**: Independent `<check>` tags, support flexible combinations
3. **Powerful data processing**: Rich built-in plugins and flexible field access
4. **Extensibility**: Support custom plugin development
5. **High-performance design**: Intelligent optimization and caching mechanisms

Remember the core philosophy: **Combine as needed, arrange flexibly**. Based on your specific requirements, freely combine various operations to create the most suitable rules.

Happy using! 🚀

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
- `condition` (required): Logical expression using check node `id`s

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

**Working Principle:**
1. Execute all check nodes with `id`, record each node's result (true/false)
2. Substitute results into `condition` expression to calculate final result
3. If final result is true, checklist passes

#### 🔍 Syntax Details: Multi-value Matching (logic and delimiter)

When you need to check if a field matches multiple values, you can use multi-value matching syntax.

**Basic Syntax:**
```xml
<check type="type" field="field" logic="OR|AND" delimiter="separator">
    value1separatorvalue2separatorvalue3
</check>
```

**Attribute Description:**
- `logic`: "OR" or "AND", specifies logical relationship between multiple values
- `delimiter`: Separator used to split multiple values

**Working Principle:**
1. Use `delimiter` to split tag content into multiple values
2. Check each value separately
3. Determine final result based on `logic`:
   - `logic="OR"`: Any value matches returns true
   - `logic="AND"`: All values must match to return true

**In the above example:**
```xml
<check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
    .exe|.dll|.scr|.bat
</check>
```
- Check if filename contains .exe, .dll, .scr, or .bat
- Use OR logic: any extension matches is sufficient
- Use | as separator

## 🔧 Part Four: Advanced Features Explained

### 4.1 Three Modes of Threshold Detection

The `<threshold>` tag not only supports simple counting, but also three powerful statistical modes:

1. **Default Mode (Counting)**: Count event occurrences
2. **SUM Mode**: Sum specified fields
3. **CLASSIFY Mode**: Count different values (deduplication counting)

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
    <threshold group_by="user,ip" range="5m" value="5"/>
    
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
               count_field="amount" value="50000"/>
    
    <append field="action">freeze_account</append>
</rule>
```

#### 🔍 Advanced Syntax: SUM Mode for threshold

**Attribute Description:**
- `count_type="SUM"`: Enable summation mode
- `count_field` (required): Field name to sum
- `value`: Trigger when cumulative sum reaches this value

**Working Principle:**
1. Group by `group_by`
2. Accumulate `count_field` values within time window
3. Trigger when cumulative value reaches `value`

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
<rule id="data_exfiltration_check" name="Data Exfiltration Detection">
    <check type="EQU" field="action">download</check>
    
    <!-- Access more than 25 different files within 1 hour -->
    <threshold group_by="user" range="1h" count_type="CLASSIFY" 
               count_field="file_id" value="25"/>
    
    <append field="risk_score">high</append>
    <plugin>alert_dlp_team(_$ORIDATA)</plugin>
</rule>
```

#### 🔍 Advanced Syntax: CLASSIFY Mode for threshold

**Attribute Description:**
- `count_type="CLASSIFY"`: Enable deduplication counting mode
- `count_field` (required): Field to count different values
- `value`: Trigger when number of different values reaches this value

**Working Principle:**
1. Group by `group_by`
2. Collect all different values of `count_field` within time window
3. Trigger when number of different values reaches `value`

**Use Cases:**
- Detect scanning behavior (access multiple different ports/IPs)
- Data exfiltration detection (access multiple different files)
- Anomaly behavior detection (use multiple different accounts)

### 4.2 Built-in Plugin System

AgentSmith-HUB provides rich built-in plugins that can be used without additional development.

#### 🧩 Complete List of Built-in Plugins

##### Check Plugins (return bool)

| Plugin Name | Function | Parameters | Example |
|-------------|----------|------------|---------|
| `isPrivateIP` | Check if IP is private address | ip (string) | `isPrivateIP(source_ip)` |
| `cidrMatch` | Check if IP is in CIDR range | ip (string), cidr (string) | `cidrMatch(client_ip, "192.168.1.0/24")` |
| `geoMatch` | Check IP country | ip (string), countryISO (string) | `geoMatch(source_ip, "US")` |
| `suppressOnce` | Alert suppression | key (any), windowSec (int), ruleid (string, optional) | `suppressOnce(alert_key, 300, "rule_001")` |

##### Data Processing Plugins (return various types)

#### Time Processing Plugins
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `now` | Get current timestamp | optional: format (unix/ms/rfc3339) | `now()` |
| `dayOfWeek` | Get day of week (0-6, 0=Sunday) | optional: timestamp (int64) | `dayOfWeek()` |
| `hourOfDay` | Get hour (0-23) | optional: timestamp (int64) | `hourOfDay()` |
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
- When referencing fields in data, no need to use `_$` prefix, use field name directly: `source_ip`
- When completely referencing all original data: `_$ORIDATA`
- When using static values, use strings directly (with quotes): `"192.168.1.0/24"`
- When using numbers, no quotes needed: `300`

## 4. Usage in Rulesets

### 4.1 Basic Usage Patterns

```xml
<root author="example" type="DETECTION" name="plugin_example">
    <rule id="plugin_usage" name="Plugin Usage Examples">
        <!-- 1. Check plugins -->
        <check type="PLUGIN">isPrivateIP(source_ip)</check>
        
        <!-- 2. Data processing plugins -->
        <append type="PLUGIN" field="timestamp">now()</append>
        <append type="PLUGIN" field="hash">hashSHA256(file_content)</append>
        
        <!-- 3. Action plugins -->
        <plugin>sendAlert(_$ORIDATA)</plugin>
    </rule>
</root>
```

### 4.2 Complex Logic Combinations

```xml
<rule id="complex_plugin_usage" name="Complex Plugin Usage">
    <!-- Use checklist to combine multiple conditions -->
    <checklist condition="(private_ip or suspicious_country) and not whitelisted">
        <check id="private_ip" type="PLUGIN">isPrivateIP(source_ip)</check>
        <check id="suspicious_country" type="PLUGIN">geoMatch(source_ip, "CN")</check>
        <check id="whitelisted" type="PLUGIN">cidrMatch(source_ip, "10.0.0.0/8")</check>
    </checklist>
    
    <!-- Data enrichment -->
    <append type="PLUGIN" field="threat_intel">virusTotal(file_hash)</append>
    <append type="PLUGIN" field="geo_info">shodan(source_ip)</append>
    
    <!-- Time-related processing -->
    <append type="PLUGIN" field="hour">hourOfDay()</append>
    <check type="PLUGIN">hourOfDay() > 22</check>
</rule>
```

### 4.3 Alert Suppression Example

```xml
<rule id="suppression_example" name="Alert Suppression">
    <check type="EQU" field="event_type">login_failed</check>
    <check type="PLUGIN">suppressOnce(source_ip, 300, "login_brute_force")</check>
    <append field="alert_type">brute_force</append>
</rule>
```

### 4.4 Data Transformation Example

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

Rules using built-in plugins:
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

Demonstrating the advantages of flexible execution order: check basic conditions first, then query threat intelligence, and finally make decisions based on results.

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
        parseJSON(_$threat_intel).severity_level
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

If using fixed execution order, this flexible processing approach of "query intelligence first, then make decisions based on results" cannot be achieved.

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
        regexExtract(_$stack_trace, "([A-Za-z.]+Exception)")
    </append>
    
    <!-- Time processing -->
    <append type="PLUGIN" field="readable_time">tsToDate(timestamp)</append>
    <append type="PLUGIN" field="hour">hourOfDay(timestamp)</append>
    
    <!-- Data masking -->
    <append type="PLUGIN" field="sanitized_message">
        regexReplace(message, "password\":\"[^\"]+", "password\":\"***")
    </append>
    
    <!-- Alert suppression: same type error only reports once in 5 minutes -->
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

**Why use the ruleid parameter?**

If the `ruleid` parameter is not used, suppression of the same key by different rules will interfere with each other:

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

#### Plugin Performance Notes

Performance levels (from high to low):
1. **Check Node Plugins**: `isPrivateIP`, `cidrMatch` - Pure computation, higher performance
2. **String Processing Plugins**: `replace`, `hashMD5/SHA1/SHA256` - Medium performance
3. **Regular Expression Plugins**: `regexExtract`, `regexReplace` - Lower performance
4. **Threat Intelligence Plugins**: `virusTotal`, `shodan`, `threatBook` - External API calls, lowest performance

Optimization suggestions:
```xml
<!-- Recommended: Use high-performance checks first, then low-performance plugins -->
<rule id="optimized">
    <check type="NOTNULL" field="required"></check>     <!-- Fastest -->
    <check type="EQU" field="type">target</check>       <!-- Fast -->
    <check type="INCL" field="message">keyword</check>  <!-- Medium -->
    <check type="REGEX" field="data">pattern</check>    <!-- Slow -->
    <check type="PLUGIN">complex_check()</check>        <!-- Slowest -->
</rule>
```

### 4.3 Whitelist Rulesets

Whitelisting is used to filter out data that does not need to be processed (ruleset type is WHITELIST). Special behavior of whitelist:
- When a whitelist rule matches, the data is "disallowed" (i.e., it is filtered out of further processing and the data is discarded).
- When all the whitelist rules do not match, the data continues to be passed to the subsequent processing.

```xml
<root type="WHITELIST" name="security_whitelist" author="security_team">
    <!-- Whitelist Rule 1: Trusted IPs -->
    <rule id="trusted_ips">
        <check type="INCL" field="source_ip" logic="OR" delimiter="|">
            10.0.0.1|10.0.0.2|10.0.0.3
        </check>
        <!-- Note: The following operations won't execute because matched data will be filtered -->
        <append field="whitelisted">true</append>
    </rule>
    
    <!-- Whitelist Rule 2: Known benign processes -->
    <rule id="benign_processes">
        <check type="INCL" field="process_name" logic="OR" delimiter="|">
            chrome.exe|firefox.exe|explorer.exe
        </check>
        <!-- Can add multiple check conditions, all must be satisfied to be filtered by whitelist -->
        <check type="PLUGIN">isPrivateIP(source_ip)</check>
    </rule>
    
    <!-- Whitelist Rule 3: Internal test traffic -->
    <rule id="test_traffic">
        <check type="INCL" field="user_agent">internal-testing-bot</check>
        <check type="PLUGIN">cidrMatch(source_ip, "192.168.100.0/24")</check>
    </rule>
</root>
``` 