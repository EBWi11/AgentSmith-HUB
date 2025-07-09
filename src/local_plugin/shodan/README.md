# Shodan Plugin

A built-in plugin for AgentSmith-HUB that queries Shodan API for IP address infrastructure information and attack surface intelligence with automatic caching.

## Features

- 🔍 Query Shodan API for IP address infrastructure information
- 🌐 Comprehensive host details including ports, services, and vulnerabilities
- 🚀 Redis-based caching (6-hour TTL) for improved performance
- 🛡️ Support for both IPv4 and IPv6 addresses
- 📍 Geographical location and ISP information
- ⚡ Automatic IP validation and normalization
- 🔒 Service fingerprinting and banner information

## Configuration

### API Key Configuration

You can configure the Shodan API key in two ways:

#### Method 1: Environment Variable (Recommended for production)

```bash
export SHODAN_API_KEY="your_api_key_here"
```

#### Method 2: Parameter (Recommended for flexibility)

Pass the API key as a second parameter to the plugin:

```xml
<append field="shodan_result" type="PLUGIN">shodan(ip_address, "your_api_key_here")</append>
```

**Note**: You can get a free API key from [Shodan](https://www.shodan.io/). Free accounts have a limit of 100 queries per month.

### Required Dependencies

- Redis (for caching)
- Internet connection to reach Shodan API

## Usage

### Basic Usage

#### With Environment Variable
```xml
<append field="shodan_result" type="PLUGIN">shodan(ip_address)</append>
```

#### With API Key Parameter
```xml
<append field="shodan_result" type="PLUGIN">shodan(ip_address, "your_api_key")</append>
```

### In Rulesets (Condition Check)

**Note**: Plugin condition checks in `<plugin>` tags are not directly supported for complex object property access. Use the plugin result in append fields instead.

```xml
<!-- Use in append to get full results -->
<append field="shodan_result" type="PLUGIN">shodan(ip_address)</append>
<append field="shodan_result" type="PLUGIN">shodan(ip_address, "api_key")</append>
```

### Examples

#### Basic IP Reconnaissance
```xml
<rule id="ip_recon">
    <filter field="event_type">network_scan</filter>
    <checklist>
        <node type="NOTNULL" field="target_ip"/>
    </checklist>
    <append field="shodan_intel" type="PLUGIN">shodan(target_ip)</append>
</rule>
```

#### With API Key Parameter
```xml
<rule id="ip_recon_with_key">
    <filter field="event_type">network_scan</filter>
    <checklist>
        <node type="NOTNULL" field="target_ip"/>
    </checklist>
    <append field="shodan_intel" type="PLUGIN">shodan(target_ip, "your_api_key")</append>
</rule>
```

#### Vulnerable Host Detection
```xml
<rule id="vuln_detection">
    <filter field="event_type">external_ip_scan</filter>
    <checklist>
        <node type="NOTNULL" field="external_ip"/>
    </checklist>
    <append field="risk_level">high</append>
    <append field="shodan_data" type="PLUGIN">shodan(external_ip, "api_key")</append>
</rule>
```

#### Attack Surface Analysis
```xml
<rule id="attack_surface">
    <filter field="event_type">asset_discovery</filter>
    <checklist>
        <node type="NOTNULL" field="public_ip"/>
    </checklist>
    <append field="exposure_risk">high</append>
    <append field="shodan_scan" type="PLUGIN">shodan(public_ip)</append>
</rule>
```

## Response Format

The plugin returns a `ShodanResult` object with the following structure:

```json
{
  "ip": "8.8.8.8",
  "hostnames": ["dns.google"],
  "location": {
    "city": "Mountain View",
    "region": "CA",
    "country": "United States",
    "country_code": "US",
    "postal_code": "94043",
    "latitude": 37.4056,
    "longitude": -122.0775
  },
  "isp": "Google LLC",
  "asn": "AS15169",
  "org": "Google Public DNS",
  "os": "Linux",
  "tags": ["cloud", "dns"],
  "vulns": ["CVE-2021-1234"],
  "last_update": "2023-12-01T10:30:00.000000",
  "ports": [53, 443, 853],
  "services": [
    {
      "port": 53,
      "transport": "udp",
      "protocol": "dns",
      "product": "Google Public DNS",
      "version": "2.0",
      "title": "Google DNS",
      "banner": "Google Public DNS",
      "timestamp": "2023-12-01T10:30:00.000000",
      "cpe": ["cpe:/a:google:dns"],
      "has_ssl": false,
      "has_http": false
    }
  ],
  "total_ports": 3,
  "has_vulns": true,
  "cached": false,
  "error": ""
}
```

### Field Descriptions

- **ip**: The queried IP address
- **hostnames**: Array of known hostnames for this IP
- **location**: Geographical information including city, country, coordinates
- **isp**: Internet Service Provider name
- **asn**: Autonomous System Number
- **org**: Organization name
- **os**: Operating system (if detected)
- **tags**: Array of tags associated with this host
- **vulns**: Array of CVE identifiers for known vulnerabilities
- **last_update**: Last time Shodan scanned this host
- **ports**: Array of open ports
- **services**: Detailed information about services running on each port
- **total_ports**: Total number of open ports
- **has_vulns**: Boolean indicating if vulnerabilities were found
- **cached**: Whether the result came from cache
- **error**: Error message if query failed

### Service Information

Each service object contains:
- **port**: Port number
- **transport**: Transport protocol (tcp/udp)
- **protocol**: Application protocol (http, ssh, dns, etc.)
- **product**: Software product name
- **version**: Software version
- **title**: Service title or description
- **banner**: Raw banner information (truncated to 500 chars)
- **timestamp**: When this service was last seen
- **cpe**: Common Platform Enumeration identifiers
- **has_ssl**: Whether SSL/TLS is detected
- **has_http**: Whether HTTP is detected

## Error Handling

The plugin handles errors gracefully:

1. **Invalid IP format**: Returns result with error field
2. **API key not configured**: Returns result with error field
3. **IP not found**: Returns result with error field
4. **API errors**: Returns result with error field
5. **Network errors**: Returns result with error field

## Caching

- **Cache Key**: `shodan_cache:{ip}`
- **TTL**: 6 hours
- **Storage**: Redis
- **Deduplication**: Automatic based on IP address

## Performance Considerations

- First query to Shodan takes ~1-3 seconds
- Cached queries return in <10ms
- Free tier: 100 queries per month
- Paid tier: Higher limits available
- Automatic retry on network errors

## Testing

Run the tests with:

```bash
cd src/local_plugin/shodan
go test -v
```

## Supported IP Formats

- **IPv4**: Standard dotted decimal notation (e.g., 192.168.1.1)
- **IPv6**: Standard colon-separated notation (e.g., 2001:db8::1)

Both public and private IP addresses are accepted, though private IPs are typically not found in Shodan.

## Rate Limits

Shodan API has rate limits:
- **Free tier**: 100 queries per month, 1 query per second
- **Small plan**: 10,000 queries per month
- **Academic plan**: 10,000 queries per month (free for students)
- **Enterprise**: Custom limits

The plugin automatically handles rate limiting through caching.

## Security Notes

- API key is read from environment variable only
- All communications use HTTPS
- No sensitive data is logged
- Cache keys are non-reversible
- Banner information is truncated to prevent excessive data storage

## Examples by Use Case

### 1. Network Asset Discovery
```xml
<!-- Basic asset discovery -->
<rule id="asset_discovery">
    <filter field="event_type">network_discovery</filter>
    <append field="shodan_scan" type="PLUGIN">shodan(discovered_ip)</append>
</rule>

<!-- With API key -->
<rule id="asset_discovery_with_key">
    <filter field="event_type">network_discovery</filter>
    <append field="shodan_scan" type="PLUGIN">shodan(discovered_ip, "your_api_key")</append>
</rule>
```

### 2. Threat Intelligence Enrichment
```xml
<rule id="ip_enrichment">
    <filter field="event_type">suspicious_ip</filter>
    <checklist>
        <node type="PLUGIN" field="suspicious_ip">shodan(_$ORIDATA, "api_key").has_vulns</node>
    </checklist>
    <append field="threat_intel" type="PLUGIN">shodan(suspicious_ip, "api_key")</append>
    <append field="risk_score">high</append>
</rule>
```

### 3. Attack Surface Analysis
```xml
<rule id="attack_surface_analysis">
    <filter field="event_type">external_scan</filter>
    <checklist>
        <node type="NOTNULL" field="public_ip"/>
    </checklist>
    <append field="exposure_level">critical</append>
    <append field="shodan_details" type="PLUGIN">shodan(public_ip)</append>
</rule>
```

### 4. Vulnerability Assessment
```xml
<rule id="vuln_assessment">
    <filter field="event_type">security_scan</filter>
    <checklist>
        <node type="NOTNULL" field="target_ip"/>
    </checklist>
    <append field="vulnerability_found">true</append>
    <append field="shodan_vulns" type="PLUGIN">shodan(target_ip, "api_key")</append>
</rule>
```

### 5. Cloud Infrastructure Monitoring
```xml
<rule id="cloud_monitoring">
    <filter field="event_type">cloud_ip_check</filter>
    <checklist>
        <node type="INCL" field="cloud_provider">aws</node>
    </checklist>
    <append field="shodan_info" type="PLUGIN">shodan(cloud_ip, "enterprise_api_key")</append>
</rule>
```

### 6. Geographic Analysis
```xml
<rule id="geo_analysis">
    <filter field="event_type">geo_intelligence</filter>
    <checklist>
        <node type="NOTNULL" field="remote_ip"/>
    </checklist>
    <append field="foreign_ip">true</append>
    <append field="geo_intel" type="PLUGIN">shodan(remote_ip)</append>
</rule>
```

### 7. Service Fingerprinting
```xml
<rule id="service_fingerprint">
    <filter field="event_type">service_discovery</filter>
    <checklist>
        <node type="NOTNULL" field="service_ip"/>
    </checklist>
    <append field="services_detected" type="PLUGIN">shodan(service_ip, "api_key")</append>
</rule>
```

### 8. Multiple API Keys for Different Environments
```xml
<!-- Production environment -->
<rule id="prod_scan">
    <filter field="environment">production</filter>
    <append field="shodan_result" type="PLUGIN">shodan(target_ip, "prod_api_key")</append>
</rule>

<!-- Development environment -->
<rule id="dev_scan">
    <filter field="environment">development</filter>
    <append field="shodan_result" type="PLUGIN">shodan(target_ip, "dev_api_key")</append>
</rule>
```

## Common Query Patterns

### Working with Plugin Results

Since the Shodan plugin returns a structured object, you need to use it in `<append>` tags to get the full results:

```xml
<!-- Basic usage: Get full Shodan information -->
<append field="shodan_result" type="PLUGIN">shodan(ip_address)</append>

<!-- With API key -->
<append field="shodan_result" type="PLUGIN">shodan(ip_address, "api_key")</append>

<!-- Use in different environments -->
<append field="prod_shodan" type="PLUGIN">shodan(ip_address, "prod_api_key")</append>
<append field="dev_shodan" type="PLUGIN">shodan(ip_address, "dev_api_key")</append>
```

### Analyzing Results

After getting the Shodan results, you can access the information in the returned object:

- `shodan_result.total_ports`: Number of open ports
- `shodan_result.has_vulns`: Boolean indicating vulnerabilities
- `shodan_result.location.country_code`: Country code
- `shodan_result.services[]`: Array of service information
- `shodan_result.vulns[]`: Array of CVE identifiers
- `shodan_result.isp`: ISP information
- `shodan_result.org`: Organization information

### Integration with Other Rules

```xml
<!-- Combine with other conditions -->
<rule id="external_asset_scan">
    <filter field="event_type">asset_discovery</filter>
    <checklist>
        <node type="NOTNULL" field="public_ip"/>
        <node type="INCL" field="source">external</node>
    </checklist>
    <append field="shodan_intel" type="PLUGIN">shodan(public_ip)</append>
    <append field="scan_timestamp" type="PLUGIN">now()</append>
</rule>
``` 