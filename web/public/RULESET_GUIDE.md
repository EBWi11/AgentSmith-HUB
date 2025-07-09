# 🛡️ AgentSmith-HUB Rules Engine Complete Guide

The AgentSmith-HUB rules engine is a powerful real-time data processing engine that can:
- 🔍 **Real-time Detection**: Identify threats and anomalies from data streams
- 🔄 **Data Transformation**: Process and enrich data
- 📊 **Statistical Analysis**: Perform threshold detection and frequency analysis
- 🚨 **Automated Response**: Trigger alerts and automated operations

### Core Concept: Flexible Execution Order

The rules engine adopts **flexible execution order** - operations are executed in the order they appear in the XML, allowing you to freely combine various operations according to your specific needs.

## 📚 Part 1: Getting Started

### 1.1 Your First Rule

Suppose we have the following data flowing in:
```json
{
   "event_type": "login",
   "username": "admin",
   "source_ip": "192.168.1.100",
   "timestamp": 1699999999
}
```

The simplest rule: detecting admin login
```xml
<root author="beginner">
   <rule id="detect_admin_login" name="Detect Admin Login">
      <!-- Standalone check, no need for checklist wrapper -->
      <check type="EQU" field="username">admin</check>

      <!-- Add marker -->
      <append field="alert">admin login detected</append>
   </rule>
</root>
```

#### 🔍 Syntax Details: `<check>` Tag

The `<check>` tag is the most basic checking unit in the rules engine, used for conditional judgment on data.

**Basic Syntax:**
```xml
<check type="check_type" field="field_name">comparison_value</check>
```

**Attribute Description:**
- `type` (required): Specifies the check type, such as `EQU` (equal), `INCL` (contains), `REGEX` (regex match), etc.
- `field` (required): The data field path to check
- Tag content: The value to compare against

**How it works:**
1. The rules engine extracts the field value specified by `field` from the input data
2. Uses the comparison method specified by `type` to compare the field value with the tag content
3. Returns true or false as the check result

#### 🔍 Syntax Details: `<append>` Tag

The `<append>` tag is used to add new fields to data or modify existing fields.

**Basic Syntax:**
```xml
<append field="field_name">value_to_add</append>
```

**Attribute Description:**
- `field` (required): The field name to add or modify
- `type` (optional): When value is "PLUGIN", indicates using a plugin to generate the value

**How it works:**
When the rule matches successfully, the `<append>` operation executes, adding the specified field and value to the data.

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

### 1.2 Adding More Check Conditions

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

Detecting admin login at unusual times:
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

#### 💡 Important Concept: Default Logic for Multiple Conditions

When a rule has multiple `<check>` tags:
- Default uses **AND** logic: all checks must pass for the rule to match
- Checks execute in order: if any check fails, subsequent checks won't execute (short-circuit evaluation)
- This design improves performance: fail early, avoid unnecessary checks

In the above example, all three check conditions must be **satisfied**:
1. username equals "admin"
2. login_time greater than 22 (after 10 PM)
3. failed_attempts greater than 3

#### 🔍 Syntax Details: `<plugin>` Tag

The `<plugin>` tag is used to execute custom operations, usually for response actions.

**Basic Syntax:**
```xml
<plugin>plugin_name(param1, param2, ...)</plugin>
```

**Features:**
- Executes operations but doesn't return values to data
- Usually used for external actions: sending alerts, executing blocks, logging, etc.
- Only executes when rule matches successfully

**Difference from `<append type="PLUGIN">`:**
- `<plugin>`: Executes operations, doesn't return values
- `<append type="PLUGIN">`: Executes plugin and adds return value to data

### 1.3 Using Dynamic Values

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

Detecting transactions exceeding user limits:
```xml
<root author="dynamic_learner">
   <rule id="over_limit_transaction" name="Over Limit Transaction Detection">
      <!-- Dynamic comparison: transaction amount > user daily limit -->
      <check type="MT" field="amount">_$user.daily_limit</check>

      <!-- Use plugin to calculate excess ratio (assuming custom plugin exists) -->
      <append type="PLUGIN" field="over_ratio">
         calculate_ratio(_$amount, _$user.daily_limit)
      </append>

      <!-- Add different handling based on VIP level -->
      <check type="EQU" field="user.vip_level">gold</check>
      <append field="action">notify_vip_service</append>
   </rule>
</root>
```

#### 🔍 Syntax Details: Dynamic Reference (_$ prefix)

The `_$` prefix is used to dynamically reference other field values in the data, rather than using static strings.

**Syntax Format:**
- `_$field_name`: Reference a single field
- `_$parent.child.grandchild`: Reference nested fields
- `_$ORIDATA`: Reference the entire original data object

**How it works:**
1. When the rules engine encounters the `_$` prefix, it recognizes it as a dynamic reference
2. Extracts the corresponding field value from the currently processed data
3. Uses the extracted value for comparison or processing

**In the above example:**
- `_$user.daily_limit` extracts the value of `user.daily_limit` from the data (5000)
- `_$amount` extracts the value of the `amount` field (10000)
- Dynamic comparison: 10000 > 5000, condition satisfied

**Common Usage:**
```xml
<!-- Dynamic comparison of two fields -->
<check type="NEQ" field="current_user">_$login_user</check>

        <!-- Use dynamic values in append -->
<append field="message">User _$username logged in from _$source_ip</append>

        <!-- Use in plugin parameters -->
<plugin>blockIP(_$malicious_ip, _$block_duration)</plugin>
```

**Using _$ORIDATA:**
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

## 📊 Part 2: Advanced Data Processing

### 2.1 Flexible Execution Order

A key feature of the rules engine is flexible execution order:

```xml
<rule id="flexible_way" name="Flexible Processing Example">
   <!-- Can add timestamp first -->
   <append type="PLUGIN" field="check_time">now()</append>

   <!-- Then perform checks -->
   <check type="EQU" field="event_type">security_event</check>

   <!-- Threshold statistics can be placed anywhere -->
   <threshold group_by="source_ip" range="5m" value="10"/>

   <!-- Continue other checks (assuming custom plugin exists) -->
   <check type="PLUGIN">is_working_hours(_$check_time)</check>

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
1. The rules engine executes operations in the order tags appear in XML
2. Check operations (check, threshold) end the rule immediately if they fail
3. Processing operations (append, del, plugin) only execute after all checks pass

#### 🔍 Syntax Details: `<threshold>` Tag

The `<threshold>` tag is used to detect the frequency of events within a specified time window.

**Basic Syntax:**
```xml
<threshold group_by="grouping_field" range="time_range" value="threshold"/>
```

**Attribute Description:**
- `group_by` (required): Which field to group statistics by, multiple fields can be comma-separated
- `range` (required): Time window, supports s(seconds), m(minutes), h(hours), d(days)
- `value` (required): Trigger threshold, check passes when this count is reached

**How it works:**
1. Group events by the `group_by` field (e.g., group by source_ip)
2. Count events for each group within the sliding time window specified by `range`
3. When a group's count reaches `value`, the check passes

**In the above example:**
- Group by source_ip
- Count events within 5 minutes
- If any IP triggers 10 times within 5 minutes, the threshold check passes

### 2.2 Complex Nested Data Processing

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
   <rule id="complex_transaction_check" name="Complex Transaction Detection">
      <!-- Check basic conditions -->
      <check type="EQU" field="request.method">POST</check>
      <check type="INCL" field="request.url">transfer</check>

      <!-- Check amount -->
      <check type="MT" field="request.body.amount">10000</check>

      <!-- Add geographic location marker -->
      <append field="geo_risk">_$request.body.metadata.geo.country</append>

      <!-- Geographic location-based threshold detection -->
      <threshold group_by="request.body.from_account,request.body.metadata.geo.country"
                 range="1h" value="3"/>

      <!-- Use plugin for deep analysis (assuming custom plugin exists) -->
      <check type="PLUGIN">analyze_transfer_risk(_$request.body)</check>

      <!-- Extract and process user-agent -->
      <append type="PLUGIN" field="client_info">parseUA(_$request.headers.user-agent)</append>

      <!-- Clean sensitive information -->
      <del>request.headers.authorization</del>
   </rule>
</root>
```

#### 🔍 Syntax Details: `<del>` Tag

The `<del>` tag is used to remove specified fields from data.

**Basic Syntax:**
```xml
<del>field1,field2,field3</del>
```

**Features:**
- Use commas to separate multiple fields
- Supports nested field paths: `user.password,session.token`
- If field doesn't exist, no error occurs, silently ignored
- Only executes when rule matches successfully

**Use Cases:**
- Remove sensitive information (passwords, tokens, keys, etc.)
- Clean temporary fields
- Reduce data size, avoid transmitting unnecessary information

### 2.3 Conditional Combination Logic

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
      <!-- Method 1: Using independent checks (default AND relationship) -->
      <check type="END" field="filename">.exe</check>
      <check type="MT" field="size">1000000</check>  <!-- Greater than 1MB -->

      <!-- Method 2: Using checklist for complex logic combinations -->
      <checklist condition="suspicious_file and (email_threat or unknown_hash)">
         <check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
            .exe|.dll|.scr|.bat
         </check>
         <check id="email_threat" type="INCL" field="sender">suspicious.com</check>
         <check id="unknown_hash" type="PLUGIN">
            is_known_malware(_$hash)
         </check>
      </checklist>

      <!-- Enrich data -->
      <append type="PLUGIN" field="virus_scan">virusTotal(_$hash)</append>
      <append field="threat_level">high</append>

      <!-- Automated response (assuming custom plugin exists) -->
      <plugin>quarantine_file(_$filename)</plugin>
      <plugin>notify_security_team(_$ORIDATA)</plugin>
   </rule>
</root>
```

#### 🔍 Syntax Details: `<checklist>` Tag

The `<checklist>` tag allows you to use custom logical expressions to combine multiple check conditions.

**Basic Syntax:**
```xml
<checklist condition="logical_expression">
   <check id="identifier1" ...>...</check>
<check id="identifier2" ...>...</check>
        </checklist>
```

**Attribute Description:**
- `condition` (required): Logical expression built using check node `id`s

**Logical Expression Syntax:**
- Use `and`, `or` to connect conditions
- Use `()` for grouping and controlling precedence
- Use `not` for negation
- Only lowercase logical operators are allowed

**Example Expressions:**
- `a and b and c`: All conditions satisfied
- `a or b or c`: Any condition satisfied
- `(a or b) and not c`: a or b satisfied, and c not satisfied
- `a and (b or (c and d))`: Complex nested conditions

**How it works:**
1. Execute all check nodes with `id`, record each node's result (true/false)
2. Substitute results into the `condition` expression to calculate final result
3. If final result is true, the checklist passes

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

**How it works:**
1. Use `delimiter` to split tag content into multiple values
2. Perform checks on each value separately
3. Determine final result based on `logic`:
   - `logic="OR"`: Returns true if any value matches
   - `logic="AND"`: Returns true only if all values match

**In the above example:**
```xml
<check id="suspicious_file" type="INCL" field="filename" logic="OR" delimiter="|">
   .exe|.dll|.scr|.bat
</check>
```
- Checks if filename contains .exe, .dll, .scr, or .bat
- Uses OR logic: any extension match is sufficient
- Uses | as separator

## 🔧 Part 3: Advanced Features

### 3.1 Three Modes of Threshold Detection

The `<threshold>` tag supports not only simple counting but also three powerful statistical modes:

1. **Default Mode (Count)**: Count event occurrences
2. **SUM Mode**: Sum specified fields
3. **CLASSIFY Mode**: Count different values (deduplication count)

#### Scenario 1: Login Failure Count Statistics (Default Count)

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

   <!-- Same user and IP failed 5 times within 5 minutes -->
   <threshold group_by="user,ip" range="5m" value="5"/>

   <append field="alert_type">brute_force_attempt</append>
   <plugin>block_ip(_$ip, 3600)</plugin>  <!-- Block for 1 hour -->
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

#### 🔍 Advanced Syntax: SUM Mode of threshold

**Attribute Description:**
- `count_type="SUM"`: Enable sum mode
- `count_field` (required): Field name to sum
- `value`: Triggers when cumulative sum reaches this value

**How it works:**
1. Group by `group_by`
2. Accumulate values of `count_field` within the time window
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

   <!-- Accessed more than 25 different files within 1 hour -->
   <threshold group_by="user" range="1h" count_type="CLASSIFY"
              count_field="file_id" value="25"/>

   <append field="risk_score">high</append>
   <plugin>alert_dlp_team(_$ORIDATA)</plugin>
</rule>
```

#### 🔍 Advanced Syntax: CLASSIFY Mode of threshold

**Attribute Description:**
- `count_type="CLASSIFY"`: Enable deduplication count mode
- `count_field` (required): Field to count different values
- `value`: Triggers when number of different values reaches this value

**How it works:**
1. Group by `group_by`
2. Collect all different values of `count_field` within the time window
3. Trigger when number of different values reaches `value`

**Use Cases:**
- Detect scanning behavior (accessing multiple different ports/IPs)
- Data exfiltration detection (accessing multiple different files)
- Anomaly detection (using multiple different accounts)

### 3.2 Built-in Plugin System

AgentSmith-HUB provides rich built-in plugins that can be used without additional development.

#### 🧩 Complete Built-in Plugin List

##### Check-Node Plugins (for conditional checks)
Can be used in `<check type="PLUGIN">` and return a boolean value. Supports negation with the `!` prefix, e.g., `<check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>` is true if the IP is not a private address.

| Plugin Name | Function | Parameters | Example |
|---|---|---|---|
| `isPrivateIP` | Check if IP is a private address | ip (string) | `<check type="PLUGIN">isPrivateIP(_$source_ip)</check>` |
| `cidrMatch` | Check if IP is in CIDR range | ip (string), cidr (string) | `<check type="PLUGIN">cidrMatch(_$client_ip, "192.168.1.0/24")</check>` |
| `geoMatch` | Check if IP belongs to specified country | ip (string), countryISO (string) | `<check type="PLUGIN">geoMatch(_$source_ip, "US")</check>` |
| `suppressOnce` | Alert suppression: trigger only once within time window | key (any), windowSec (int), ruleid (string, optional) | `<check type="PLUGIN">suppressOnce(_$alert_key, 300, "rule_001")</check>` |

**Note on plugin parameter format**:
- When referencing fields in data, use `_$` prefix: `_$source_ip`
- When using static values, use strings directly (with quotes): `"192.168.1.0/24"`
- When using numbers, no quotes needed: `300`

##### Data Processing Plugins (for data transformation)
Can be used in `<append type="PLUGIN">`, returns various types of values:

**Time Processing Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `now` | Get current timestamp | optional: format (unix/ms/rfc3339) | `<append type="PLUGIN" field="timestamp">now()</append>` |
| `dayOfWeek` | Get day of week (0-6, 0=Sunday) | optional: timestamp (int64) | `<append type="PLUGIN" field="weekday">dayOfWeek()</append>` |
| `hourOfDay` | Get hour (0-23) | optional: timestamp (int64) | `<append type="PLUGIN" field="hour">hourOfDay()</append>` |

**Encoding and Hash Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `base64Encode` | Base64 encoding | input (string) | `<append type="PLUGIN" field="encoded">base64Encode(_$raw_data)</append>` |
| `base64Decode` | Base64 decoding | encoded (string) | `<append type="PLUGIN" field="decoded">base64Decode(_$encoded_data)</append>` |
| `hashMD5` | Calculate MD5 hash | input (string) | `<append type="PLUGIN" field="md5">hashMD5(_$password)</append>` |
| `hashSHA1` | Calculate SHA1 hash | input (string) | `<append type="PLUGIN" field="sha1">hashSHA1(_$content)</append>` |
| `hashSHA256` | Calculate SHA256 hash | input (string) | `<append type="PLUGIN" field="sha256">hashSHA256(_$file_data)</append>` |

**URL Processing Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `extractDomain` | Extract domain from URL | urlOrHost (string) | `<append type="PLUGIN" field="domain">extractDomain(_$request_url)</append>` |
| `extractTLD` | Extract top-level domain | domain (string) | `<append type="PLUGIN" field="tld">extractTLD(_$hostname)</append>` |
| `extractSubdomain` | Extract subdomain from hostname | host (string) | `<append type="PLUGIN" field="subdomain">extractSubdomain(_$full_hostname)</append>` |

**String Processing Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `replace` | String replacement | input (string), old (string), new (string) | `<append type="PLUGIN" field="cleaned">replace(_$raw_text, "bad", "good")</append>` |
| `regexExtract` | Regex extraction | input (string), pattern (string) | `<append type="PLUGIN" field="extracted">regexExtract(_$log_line, "IP: (\\d+\\.\\d+\\.\\d+\\.\\d+)")</append>` |
| `regexReplace` | Regex replacement | input (string), pattern (string), replacement (string) | `<append type="PLUGIN" field="masked">regexReplace(_$email, "(.+)@(.+)", "$1@***")</append>` |

**Data Parsing Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `parseJSON` | Parse JSON string | jsonString (string) | `<append type="PLUGIN" field="parsed">parseJSON(_$json_data)</append>` |
| `parseUA` | Parse User-Agent | userAgent (string) | `<append type="PLUGIN" field="browser_info">parseUA(_$user_agent)</append>` |

**Threat Intelligence Plugins**
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| `virusTotal` | Query VirusTotal file hash threat intelligence | hash (string), apiKey (string, optional) | `<append type="PLUGIN" field="vt_scan">virusTotal(_$file_hash)</append>` |
| `shodan` | Query Shodan IP address infrastructure intelligence | ip (string), apiKey (string, optional) | `<append type="PLUGIN" field="shodan_intel">shodan(_$ip_address)</append>` |
| `threatBook` | Query ThreatBook threat intelligence | queryValue (string), queryType (string), apiKey (string, optional) | `<append type="PLUGIN" field="tb_intel">threatBook(_$target_ip, "ip")</append>` |

**Threat Intelligence Plugin Configuration Notes**:
- API Keys can be set uniformly in configuration files or passed when calling plugins
- Some features may be limited without API Keys
- Recommend managing API Keys uniformly in system configuration to avoid hardcoding in rules

### 3.3 Whitelist Rulesets

Whitelists are used to filter out data that doesn't need processing. Special behavior of whitelists:
- When a whitelist rule matches, data is "allowed through" (i.e., filtered out, no further processing)
- When a whitelist rule doesn't match, data continues to be passed to subsequent processing
- `append`, `del`, `plugin` operations in whitelists won't execute (because matched data is filtered)

```xml
<root type="WHITELIST" name="security_whitelist" author="security_team">
    <!-- Whitelist rule 1: Trusted IPs -->
    <rule id="trusted_ips">
        <check type="INCL" field="source_ip" logic="OR" delimiter="|">
            10.0.0.1|10.0.0.2|10.0.0.3
        </check>
        <!-- Note: Following operations won't execute because matched data is filtered -->
        <append field="whitelisted">true</append>
    </rule>
    
    <!-- Whitelist rule 2: Known benign processes -->
    <rule id="benign_processes">
        <check type="INCL" field="process_name" logic="OR" delimiter="|">
            chrome.exe|firefox.exe|explorer.exe
        </check>
        <!-- Can add multiple check conditions, all must be satisfied for whitelist filtering -->
        <check type="PLUGIN">isPrivateIP(_$source_ip)</check>
    </rule>
    
    <!-- Whitelist rule 3: Internal test traffic -->
    <rule id="test_traffic">
        <check type="INCL" field="user_agent">internal-testing-bot</check>
        <check type="PLUGIN">cidrMatch(_$source_ip, "192.168.100.0/24")</check>
    </rule>
</root>
```

## 🚨 Part 4: Practical Case Studies

### Case 1: APT Attack Detection

Complete APT attack detection ruleset (using built-in plugins and assumed custom plugins):

```xml
<root type="DETECTION" name="apt_detection_suite" author="threat_hunting_team">
    <!-- Rule 1: PowerShell Empire Detection -->
    <rule id="powershell_empire" name="PowerShell Empire C2 Detection">
        <!-- Flexible order: enrichment first, then detection -->
        <append type="PLUGIN" field="decoded_cmd">base64Decode(_$command_line)</append>
        
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
            regexExtract(_$decoded_cmd, "https?://[^\\s]+")
        </append>
        
        <!-- Generate IoC -->
        <append field="ioc_type">powershell_empire_c2</append>
        <append type="PLUGIN" field="ioc_hash">hashSHA256(_$decoded_cmd)</append>
        
        <!-- Automated response (assuming custom plugins exist) -->
        <plugin>isolateHost(_$hostname)</plugin>
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
            <!-- Exclude internal scans -->
            <check id="internal_scan" type="PLUGIN">
                isPrivateIP(_$source_ip)
            </check>
        </checklist>
        
        <!-- Time window detection -->
        <threshold group_by="source_ip,dest_ip" range="30m" value="5"/>
        
        <!-- Risk scoring (assuming custom plugin exists) -->
        <append type="PLUGIN" field="risk_score">
            calculateLateralMovementRisk(_$ORIDATA)
        </append>
        
        <plugin>updateAttackGraph(_$source_ip, _$dest_ip)</plugin>
    </rule>
    
    <!-- Rule 3: Data Exfiltration Detection -->
    <rule id="data_exfiltration" name="Data Exfiltration Detection">
        <!-- First check if it's sensitive data access -->
        <check type="INCL" field="file_path" logic="OR" delimiter="|">
            /etc/passwd|/etc/shadow|.ssh/|.aws/credentials
        </check>

        <!-- Check external connection behavior -->
        <check type="PLUGIN">!isPrivateIP(_$dest_ip)</check>
        
        <!-- Abnormal transfer detection -->
        <threshold group_by="source_ip" range="1h" count_type="SUM" 
                   count_field="bytes_sent" value="1073741824"/>  <!-- 1GB -->
        
        <!-- DNS tunnel detection (assuming custom plugin exists) -->
        <checklist condition="dns_tunnel_check">
            <check id="dns_tunnel_check" type="PLUGIN">
                detectDNSTunnel(_$dns_queries)
            </check>
        </checklist>
        
        <!-- Generate alert -->
        <append field="alert_severity">critical</append>
        <append type="PLUGIN" field="data_classification">
            classifyData(_$file_path)
        </append>
        
        <plugin>blockDataTransfer(_$source_ip, _$dest_ip)</plugin>
        <plugin>triggerIncidentResponse(_$ORIDATA)</plugin>
    </rule>
</root>
```

### Case 2: Real-time Financial Fraud Detection

```xml
<root type="DETECTION" name="fraud_detection_system" author="risk_team">
    <!-- Rule 1: Account Takeover Detection -->
    <rule id="account_takeover" name="Account Takeover Detection">
        <!-- Real-time device fingerprinting (assuming custom plugin exists) -->
        <append type="PLUGIN" field="device_fingerprint">
            generateFingerprint(_$user_agent, _$screen_resolution, _$timezone)
        </append>
        
        <!-- Check device change (assuming custom plugin exists) -->
        <check type="PLUGIN">
            isNewDevice(_$user_id, _$device_fingerprint)
        </check>
        
        <!-- Geographic location anomaly (assuming custom plugin exists) -->
        <append type="PLUGIN" field="geo_distance">
            calculateGeoDistance(_$user_id, _$current_ip, _$last_ip)
        </append>
        <check type="MT" field="geo_distance">500</check>  <!-- 500km -->
        
        <!-- Behavior pattern analysis (assuming custom plugin exists) -->
        <append type="PLUGIN" field="behavior_score">
            analyzeBehaviorPattern(_$user_id, _$recent_actions)
        </append>
        
        <!-- Transaction speed detection -->
        <threshold group_by="user_id" range="10m" value="5"/>
        
        <!-- Risk decision (assuming custom plugin exists) -->
        <append type="PLUGIN" field="risk_decision">
            makeRiskDecision(_$behavior_score, _$geo_distance, _$device_fingerprint)
        </append>
        
        <!-- Real-time intervention (assuming custom plugin exists) -->
        <plugin>requireMFA(_$user_id, _$transaction_id)</plugin>
        <plugin>notifyUser(_$user_id, "suspicious_activity")</plugin>
    </rule>
    
    <!-- Rule 2: Money Laundering Detection -->
    <rule id="money_laundering" name="Money Laundering Detection">
        <!-- Scatter-gather pattern (assuming custom plugin exists) -->
        <checklist condition="structuring or layering or integration">
            <!-- Structuring -->
            <check id="structuring" type="PLUGIN">
                detectStructuring(_$user_id, _$transaction_history)
            </check>
            <!-- Layering -->
            <check id="layering" type="PLUGIN">
                detectLayering(_$account_network, _$transaction_flow)
            </check>
            <!-- Integration -->
            <check id="integration" type="PLUGIN">
                detectIntegration(_$merchant_category, _$transaction_pattern)
            </check>
        </checklist>
        
        <!-- Correlation analysis (assuming custom plugin exists) -->
        <append type="PLUGIN" field="network_risk">
            analyzeAccountNetwork(_$user_id, _$connected_accounts)
        </append>
        
        <!-- Cumulative amount monitoring -->
        <threshold group_by="account_cluster" range="7d" count_type="SUM"
                   count_field="amount" value="1000000"/>
        
        <!-- Compliance reporting (assuming custom plugin exists) -->
        <append type="PLUGIN" field="sar_report">
            generateSAR(_$ORIDATA)  <!-- Suspicious Activity Report -->
        </append>
        
        <plugin>freezeAccountCluster(_$account_cluster)</plugin>
        <plugin>notifyCompliance(_$sar_report)</plugin>
    </rule>
</root>
```

## 📖 Part 5: Syntax Reference Manual

### 5.1 Ruleset Structure

#### Root Element `<root>`
```xml
<root type="DETECTION|WHITELIST" name="ruleset_name" author="author">
    <!-- Rule list -->
</root>
```

| Attribute | Required | Description | Default |
|-----------|----------|-------------|---------|
| type | No | Ruleset type | DETECTION |
| name | No | Ruleset name | - |
| author | No | Author information | - |

#### Rule Element `<rule>`
```xml
<rule id="unique_identifier" name="rule_description">
    <!-- Operation list: executed in order of appearance -->
</rule>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| id | Yes | Unique rule identifier |
| name | No | Readable rule description |

### 5.2 Check Operations

#### Standalone Check `<check>`
```xml
<check type="type" field="field_name" logic="OR|AND" delimiter="separator">
    value
</check>
```

| Attribute | Required | Description | Use Case |
|-----------|----------|-------------|----------|
| type | Yes | Check type | All |
| field | Conditional | Field name (optional for PLUGIN type) | Required for non-PLUGIN types |
| logic | No | Multi-value logic | When using delimiter |
| delimiter | Conditional | Value separator | Required when using logic |
| id | Conditional | Node identifier | Required when using condition in checklist |

#### Checklist `<checklist>`
```xml
<checklist condition="logical_expression">
    <check id="a" ...>...</check>
    <check id="b" ...>...</check>
</checklist>
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| condition | No | Logical expression (e.g., `a and (b or c)`) |

### 5.3 Complete Check Types List

#### String Matching Types
| Type | Description | Case Sensitive | Example |
|------|-------------|----------------|---------|
| EQU | Exact match | Yes | `<check type="EQU" field="status">active</check>` |
| NEQ | Not equal | Yes | `<check type="NEQ" field="status">inactive</check>` |
| INCL | Contains substring | Yes | `<check type="INCL" field="message">error</check>` |
| NI | Does not contain | Yes | `<check type="NI" field="message">success</check>` |
| START | Starts with | Yes | `<check type="START" field="path">/admin</check>` |
| END | Ends with | Yes | `<check type="END" field="file">.exe</check>` |
| NSTART | Does not start with | Yes | `<check type="NSTART" field="path">/public</check>` |
| NEND | Does not end with | Yes | `<check type="NEND" field="file">.txt</check>` |

#### Case Insensitive Types
| Type | Description | Example |
|------|-------------|---------|
| NCS_EQU | Case insensitive equal | `<check type="NCS_EQU" field="protocol">HTTP</check>` |
| NCS_NEQ | Case insensitive not equal | `<check type="NCS_NEQ" field="method">get</check>` |
| NCS_INCL | Case insensitive contains | `<check type="NCS_INCL" field="header">Content-Type</check>` |
| NCS_NI | Case insensitive does not contain | `<check type="NCS_NI" field="useragent">bot</check>` |
| NCS_START | Case insensitive starts with | `<check type="NCS_START" field="domain">WWW.</check>` |
| NCS_END | Case insensitive ends with | `<check type="NCS_END" field="email">.COM</check>` |
| NCS_NSTART | Case insensitive does not start with | `<check type="NCS_NSTART" field="url">HTTP://</check>` |
| NCS_NEND | Case insensitive does not end with | `<check type="NCS_NEND" field="filename">.EXE</check>` |

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
| PLUGIN | Plugin function (supports `!` negation) | `<check type="PLUGIN">isValidEmail(_$email)</check>` |

### 5.4 Data Processing Operations

#### Threshold Detection `<threshold>`
```xml
<threshold group_by="field1,field2" range="time_range" value="threshold"
           count_type="SUM|CLASSIFY" count_field="count_field" local_cache="true|false"/>
```

| Attribute | Required | Description | Example |
|-----------|----------|-------------|---------|
| group_by | Yes | Grouping fields | `source_ip,user_id` |
| range | Yes | Time range | `5m`, `1h`, `24h` |
| value | Yes | Threshold value | `10` |
| count_type | No | Count type | Default: count, `SUM`: sum, `CLASSIFY`: dedup count |
| count_field | Conditional | Count field | Required when using SUM/CLASSIFY |
| local_cache | No | Use local cache | `true` or `false` |

#### Field Append `<append>`
```xml
<append field="field_name" type="PLUGIN">value_or_plugin_call</append>
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
<plugin>plugin_function(param1, param2)</plugin>
```

### 5.5 Field Access Syntax

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

### 5.6 Built-in Plugin Quick Reference

#### Check Plugins (return bool)
| Plugin | Function | Parameters | Example |
|--------|----------|------------|---------|
| isPrivateIP | Check private IP | ip | `isPrivateIP(_$ip)` |
| cidrMatch | CIDR matching | ip, cidr | `cidrMatch(_$ip, "10.0.0.0/8")` |
| geoMatch | Geographic matching | ip, country | `geoMatch(_$ip, "US")` |
| suppressOnce | Alert suppression | key, seconds, ruleid | `suppressOnce(_$ip, 300, "rule1")` |

#### Data Processing Plugins (return various types)
| Plugin | Function | Return Type | Example |
|--------|----------|-------------|---------|
| now | Current time | int64 | `now()` |
| base64Encode | Base64 encoding | string | `base64Encode(_$data)` |
| hashSHA256 | SHA256 hash | string | `hashSHA256(_$content)` |
| parseJSON | JSON parsing | object | `parseJSON(_$json_str)` |
| regexExtract | Regex extraction | string | `regexExtract(_$text, pattern)` |

### 5.7 Performance Optimization Recommendations

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
<!-- Use local cache for better performance -->
<threshold group_by="user_id" range="5m" value="10" local_cache="true"/>

        <!-- Avoid overly large time windows -->
<threshold group_by="ip" range="1h" value="1000"/>  <!-- Don't exceed 24h -->
```

### 5.8 Common Errors and Solutions

#### XML Syntax Errors
```xml
<!-- Wrong: Special characters not escaped -->
<check type="INCL" field="xml"><tag>value</tag></check>

        <!-- Correct: Use CDATA -->
<check type="INCL" field="xml"><![CDATA[<tag>value</tag>]]></check>
```

#### Logic Errors
```xml
<!-- Wrong: condition references non-existent id -->
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
<!-- Problem: Using plugins directly on large amounts of data -->
<rule id="slow">
   <check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>

        <!-- Optimized: Filter first, then process -->
<rule id="fast">
<check type="EQU" field="type">target</check>
<check type="PLUGIN">expensive_check(_$ORIDATA)</check>
</rule>
```

## 🎯 Summary

Core advantages of the AgentSmith-HUB rules engine:

1. **Completely Flexible Execution Order**: Operations execute in the order they appear in XML
2. **Concise Syntax**: Independent `<check>` tags support flexible combinations
3. **Powerful Data Processing**: Rich built-in plugins and flexible field access
4. **Extensibility**: Support for custom plugin development
5. **High-Performance Design**: Intelligent optimization and caching mechanisms

Remember the core concept: **Combine as needed, arrange flexibly**. According to your specific requirements, freely combine various operations to create the most suitable rules.

Happy using! 🚀
