# ThreatBook Plugin (微步在线插件)

A built-in plugin for AgentSmith-HUB that queries ThreatBook (微步在线) API for comprehensive threat intelligence with automatic caching. Supports multiple query types including IP addresses, domains, file hashes, and URLs.

## Features

- 🌐 **Multi-Type Queries**: Support IP, domain, file hash, and URL threat intelligence
- 🔍 **Comprehensive Intelligence**: Rich threat information including malicious indicators, threat types, and confidence levels
- 🇨🇳 **Chinese Threat Intelligence**: Leading Chinese threat intelligence platform with local context
- 🚀 **Redis-based Caching** (12-hour TTL) for improved performance
- 📍 **Geographical Information**: Location data for IP addresses with Chinese regional details
- 🏢 **ASN Information**: Autonomous System Number details for IP queries
- 🔒 **File Hash Analysis**: Support for MD5, SHA1, and SHA256 hashes
- ⚡ **Automatic Validation**: Input validation for all query types

## Configuration

### API Key Configuration

You can configure the ThreatBook API key in two ways:

#### Method 1: Environment Variable (Recommended for production)

```bash
export THREATBOOK_API_KEY="your_api_key_here"
```

#### Method 2: Parameter (Recommended for flexibility)

Pass the API key as a third parameter to the plugin:

```xml
<append field="threat_result" type="PLUGIN">threatBook(target_ip, "ip", "your_api_key_here")</append>
```

**Note**: You can get an API key from [微步在线](https://x.threatbook.cn/). Registration may require Chinese phone number verification.

### Required Dependencies

- Redis (for caching)
- Internet connection to reach ThreatBook API

## Usage

### Basic Usage

#### With Environment Variable
```xml
<append field="threat_result" type="PLUGIN">threatBook(query_value, query_type)</append>
```

#### With API Key Parameter
```xml
<append field="threat_result" type="PLUGIN">threatBook(query_value, query_type, "your_api_key")</append>
```

### Supported Query Types

- **`ip`**: IP address threat intelligence (IPv4/IPv6)
- **`domain`**: Domain reputation and threat analysis
- **`file`**: File hash reputation (MD5/SHA1/SHA256)
- **`url`**: URL reputation and safety analysis

### Examples

#### IP Address Intelligence
```xml
<rule id="ip_threat_check">
    <filter field="event_type">network_traffic</filter>
    <checklist>
        <node type="NOTNULL" field="src_ip"/>
    </checklist>
    <append field="src_ip_intel" type="PLUGIN">threatBook(src_ip, "ip")</append>
</rule>
```

#### Domain Reputation Check
```xml
<rule id="domain_reputation">
    <filter field="event_type">dns_query</filter>
    <checklist>
        <node type="NOTNULL" field="domain"/>
    </checklist>
    <append field="domain_intel" type="PLUGIN">threatBook(domain, "domain", "your_api_key")</append>
</rule>
```

#### File Hash Analysis
```xml
<rule id="file_threat_analysis">
    <filter field="event_type">file_scan</filter>
    <checklist>
        <node type="NOTNULL" field="file_hash"/>
    </checklist>
    <append field="file_intel" type="PLUGIN">threatBook(file_hash, "file")</append>
</rule>
```

#### URL Safety Check
```xml
<rule id="url_safety_check">
    <filter field="event_type">web_access</filter>
    <checklist>
        <node type="NOTNULL" field="request_url"/>
    </checklist>
    <append field="url_intel" type="PLUGIN">threatBook(request_url, "url", "your_api_key")</append>
</rule>
```

## Response Format

The plugin returns a `ThreatBookResult` object with the following structure:

```json
{
  "query_value": "8.8.8.8",
  "query_type": "ip",
  "response_code": 0,
  "is_malicious": false,
  "threat_types": [],
  "confidence": "high",
  "severity": "low",
  "tags": [],
  "intel_types": ["basic_info"],
  "location": {
    "country": "美国",
    "province": "加利福尼亚州",
    "city": "山景城",
    "country_code": "US",
    "longitude": -122.0838,
    "latitude": 37.4056
  },
  "asn": {
    "number": 15169,
    "info": "GOOGLE",
    "rank": "1"
  },
  "update_time": "2024-01-15 10:30:00",
  "summary": {
    "basic_info": "Google Public DNS"
  },
  "intelligence": {
    "confidence": 90,
    "source": "threatbook"
  },
  "context": {
    "last_seen": "2024-01-15",
    "first_seen": "2023-01-01"
  },
  "cached": false,
  "error": ""
}
```

### Field Descriptions

#### Core Information
- **query_value**: The original query value
- **query_type**: Type of query (ip/domain/file/url)
- **response_code**: API response code (0 = success)
- **is_malicious**: Boolean indicating if the target is malicious
- **threat_types**: Array of threat classifications
- **confidence**: Confidence level of the assessment
- **severity**: Threat severity level
- **tags**: Associated threat tags
- **intel_types**: Types of intelligence available

#### IP-Specific Information
- **location**: Geographical information including Chinese localization
  - **country**: Country name (in Chinese)
  - **province**: Province/state name
  - **city**: City name
  - **country_code**: ISO country code
  - **longitude/latitude**: GPS coordinates
- **asn**: Autonomous System Number information
  - **number**: ASN number
  - **info**: ASN organization info
  - **rank**: ASN ranking

#### File-Specific Information
- **file_hashes**: Hash information for files
  - **md5**: MD5 hash
  - **sha1**: SHA1 hash
  - **sha256**: SHA256 hash

#### Additional Information
- **update_time**: Last update timestamp
- **summary**: Summary information object
- **intelligence**: Intelligence metadata
- **context**: Additional context information
- **cached**: Whether result came from cache
- **error**: Error message if query failed

## Error Handling

The plugin handles errors gracefully:

1. **Invalid query type**: Returns result with error field
2. **Invalid query value format**: Returns result with error field
3. **API key not configured**: Returns result with error field
4. **API errors**: Returns result with error field
5. **Network errors**: Returns result with error field

## Caching

- **Cache Key**: `threatbook_cache:{query_type}:{query_value}`
- **TTL**: 12 hours
- **Storage**: Redis
- **Deduplication**: Automatic based on query type and value

## Performance Considerations

- First query to ThreatBook takes ~2-5 seconds
- Cached queries return in <10ms
- Different query types may have different response times
- Automatic retry on network errors

## Testing

Run the tests with:

```bash
cd src/local_plugin/threatbook
go test -v
```

## Supported Formats

### IP Addresses
- **IPv4**: Standard dotted decimal notation (e.g., 192.168.1.1)
- **IPv6**: Standard colon-separated notation (e.g., 2001:db8::1)

### Domains
- **Standard domains**: example.com, sub.example.com
- **International domains**: 中国.cn (IDN domains)

### File Hashes
- **MD5**: 32 hexadecimal characters
- **SHA1**: 40 hexadecimal characters
- **SHA256**: 64 hexadecimal characters

### URLs
- **HTTP/HTTPS URLs**: Full URLs with protocol

## Rate Limits

ThreatBook API has rate limits based on your account tier:
- **Free tier**: Limited queries per day
- **Paid tier**: Higher limits available
- **Enterprise**: Custom limits

The plugin automatically handles rate limiting through caching.

## Security Notes

- API key is read from environment variable by default
- All communications use HTTPS
- No sensitive data is logged
- Cache keys are non-reversible
- Input validation prevents injection attacks

## Examples by Use Case

### 1. Network Security Monitoring
```xml
<!-- IP reputation check -->
<rule id="ip_reputation">
    <filter field="event_type">network_connection</filter>
    <checklist>
        <node type="NOTNULL" field="dst_ip"/>
    </checklist>
    <append field="dst_ip_intel" type="PLUGIN">threatBook(dst_ip, "ip")</append>
</rule>

<!-- With API key for production -->
<rule id="ip_reputation_prod">
    <filter field="environment">production</filter>
    <append field="ip_intel" type="PLUGIN">threatBook(remote_ip, "ip", "prod_api_key")</append>
</rule>
```

### 2. Email Security
```xml
<!-- Domain reputation for email sender -->
<rule id="email_sender_domain">
    <filter field="event_type">email_received</filter>
    <checklist>
        <node type="NOTNULL" field="sender_domain"/>
    </checklist>
    <append field="sender_reputation" type="PLUGIN">threatBook(sender_domain, "domain", "email_api_key")</append>
</rule>

<!-- URL analysis in email content -->
<rule id="email_url_check">
    <filter field="event_type">email_analysis</filter>
    <checklist>
        <node type="NOTNULL" field="extracted_url"/>
    </checklist>
    <append field="url_reputation" type="PLUGIN">threatBook(extracted_url, "url")</append>
</rule>
```

### 3. File Security Analysis
```xml
<!-- Malware detection -->
<rule id="file_malware_check">
    <filter field="event_type">file_upload</filter>
    <checklist>
        <node type="NOTNULL" field="file_sha256"/>
    </checklist>
    <append field="malware_scan" type="PLUGIN">threatBook(file_sha256, "file", "security_api_key")</append>
</rule>

<!-- Multi-hash analysis -->
<rule id="comprehensive_file_check">
    <filter field="event_type">file_analysis</filter>
    <checklist>
        <node type="NOTNULL" field="file_md5"/>
    </checklist>
    <append field="md5_intel" type="PLUGIN">threatBook(file_md5, "file")</append>
    <append field="sha1_intel" type="PLUGIN">threatBook(file_sha1, "file")</append>
    <append field="sha256_intel" type="PLUGIN">threatBook(file_sha256, "file")</append>
</rule>
```

### 4. Web Security
```xml
<!-- Suspicious domain detection -->
<rule id="suspicious_domain">
    <filter field="event_type">dns_query</filter>
    <checklist>
        <node type="INCL" field="query_name">suspicious</node>
    </checklist>
    <append field="domain_analysis" type="PLUGIN">threatBook(query_name, "domain")</append>
</rule>

<!-- Phishing URL detection -->
<rule id="phishing_detection">
    <filter field="event_type">web_request</filter>
    <checklist>
        <node type="INCL" field="url">login</node>
    </checklist>
    <append field="phishing_check" type="PLUGIN">threatBook(url, "url", "web_security_key")</append>
</rule>
```

### 5. Threat Hunting
```xml
<!-- APT infrastructure detection -->
<rule id="apt_infrastructure">
    <filter field="event_type">threat_hunting</filter>
    <checklist>
        <node type="NOTNULL" field="ioc_value"/>
        <node type="NOTNULL" field="ioc_type"/>
    </checklist>
    <append field="apt_intel" type="PLUGIN">threatBook(ioc_value, ioc_type, "threat_hunting_key")</append>
</rule>

<!-- Command and control detection -->
<rule id="c2_detection">
    <filter field="event_type">network_analysis</filter>
    <checklist>
        <node type="NOTNULL" field="c2_domain"/>
    </checklist>
    <append field="c2_intel" type="PLUGIN">threatBook(c2_domain, "domain")</append>
    <append field="c2_ip_intel" type="PLUGIN">threatBook(c2_ip, "ip")</append>
</rule>
```

### 6. Multi-Environment Support
```xml
<!-- Development environment -->
<rule id="dev_threat_check">
    <filter field="environment">development</filter>
    <append field="threat_intel" type="PLUGIN">threatBook(indicator, indicator_type, "dev_api_key")</append>
</rule>

<!-- Staging environment -->
<rule id="staging_threat_check">
    <filter field="environment">staging</filter>
    <append field="threat_intel" type="PLUGIN">threatBook(indicator, indicator_type, "staging_api_key")</append>
</rule>

<!-- Production environment -->
<rule id="prod_threat_check">
    <filter field="environment">production</filter>
    <append field="threat_intel" type="PLUGIN">threatBook(indicator, indicator_type, "prod_api_key")</append>
</rule>
```

### 7. Automated Response
```xml
<!-- High-confidence malicious IP blocking -->
<rule id="auto_ip_block">
    <filter field="event_type">security_alert</filter>
    <checklist>
        <node type="NOTNULL" field="suspicious_ip"/>
    </checklist>
    <append field="ip_analysis" type="PLUGIN">threatBook(suspicious_ip, "ip")</append>
    <append field="action">investigate</append>
</rule>

<!-- Domain-based alerting -->
<rule id="domain_alert">
    <filter field="event_type">dns_analysis</filter>
    <checklist>
        <node type="NOTNULL" field="queried_domain"/>
    </checklist>
    <append field="domain_threat" type="PLUGIN">threatBook(queried_domain, "domain", "alerting_key")</append>
</rule>
```

## Working with Plugin Results

After getting the ThreatBook results, you can use the structured data:

- `threat_result.is_malicious`: Boolean for quick threat detection
- `threat_result.threat_types[]`: Array of threat classifications
- `threat_result.confidence`: Confidence level assessment
- `threat_result.location.country`: Geographic location (for IP queries)
- `threat_result.asn.info`: ISP/Organization information
- `threat_result.file_hashes.*`: Multiple hash formats (for file queries)

### Integration with Other Security Tools

```xml
<!-- Combine with other threat intelligence -->
<rule id="multi_source_intel">
    <filter field="event_type">threat_analysis</filter>
    <checklist>
        <node type="NOTNULL" field="indicator"/>
    </checklist>
    <append field="threatbook_intel" type="PLUGIN">threatBook(indicator, "ip")</append>
    <append field="virustotal_intel" type="PLUGIN">virusTotal(file_hash)</append>
    <append field="shodan_intel" type="PLUGIN">shodan(indicator)</append>
    <append field="analysis_timestamp" type="PLUGIN">now()</append>
</rule>
```

## Chinese Localization Features

ThreatBook provides excellent support for Chinese threat intelligence:

- **Chinese threat actor tracking**: Local APT groups and campaigns
- **Chinese malware families**: Specialized detection for China-focused threats
- **Regional context**: Understanding of Chinese internet infrastructure
- **Language support**: Chinese threat descriptions and classifications
- **Local compliance**: Adherence to Chinese cybersecurity regulations

This makes the ThreatBook plugin particularly valuable for organizations operating in or dealing with threats related to the Chinese cybersecurity landscape. 