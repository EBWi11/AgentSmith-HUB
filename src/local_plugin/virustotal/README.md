# VirusTotal Plugin

A built-in plugin for AgentSmith-HUB that queries VirusTotal API for file hash reputation with automatic caching.

## Features

- 🔍 Query VirusTotal API for file hash reputation
- 📊 Comprehensive detection information from multiple antivirus engines
- 🚀 Redis-based caching (24-hour TTL) for improved performance
- 🛡️ Support for MD5, SHA1, and SHA256 hashes
- 📝 Detailed result structure with engine-specific detections
- ⚡ Automatic hash validation and normalization

## Configuration

### API Key Configuration

You can configure the VirusTotal API key in two ways:

#### Method 1: Environment Variable (Recommended for production)

```bash
export VIRUSTOTAL_API_KEY="your_api_key_here"
```

#### Method 2: Parameter (Recommended for flexibility)

Pass the API key as a second parameter to the plugin:

```xml
<append field="vt_result" type="PLUGIN">virusTotal(file_hash, "your_api_key_here")</append>
```

**Note**: You can get a free API key from [VirusTotal](https://www.virustotal.com/gui/join-us).

### Required Dependencies

- Redis (for caching)
- Internet connection to reach VirusTotal API

## Usage

### Basic Usage

#### With Environment Variable
```xml
<append field="vt_result" type="PLUGIN">virusTotal(file_hash)</append>
```

#### With API Key Parameter
```xml
<append field="vt_result" type="PLUGIN">virusTotal(file_hash, "your_api_key")</append>
```

### In Rulesets (Condition Check)

```xml
<plugin>virusTotal(file_hash).detections > 0</plugin>
<plugin>virusTotal(file_hash, "api_key").malicious > 5</plugin>
```

### Examples

#### Basic Usage
```xml
<rule id="vt_check">
    <filter field="event_type">file_hash</filter>
    <checklist>
        <node type="NOTNULL" field="md5_hash"/>
    </checklist>
    <append field="vt_intel" type="PLUGIN">virusTotal(md5_hash)</append>
</rule>
```

#### With API Key Parameter
```xml
<rule id="vt_check_with_key">
    <filter field="event_type">file_hash</filter>
    <checklist>
        <node type="NOTNULL" field="md5_hash"/>
    </checklist>
    <append field="vt_intel" type="PLUGIN">virusTotal(md5_hash, "your_api_key")</append>
</rule>
```

#### Malware Detection
```xml
<rule id="malware_detection">
    <filter field="event_type">file_scan</filter>
    <checklist>
        <node type="PLUGIN" field="sha256_hash">virusTotal(_$ORIDATA, "api_key").malicious > 5</node>
    </checklist>
    <append field="threat_level">high</append>
</rule>
```

#### Conditional Processing
```xml
<rule id="suspicious_file">
    <filter field="event_type">file_upload</filter>
    <checklist>
        <node type="PLUGIN" field="file_hash">virusTotal(_$ORIDATA).detections > 0</node>
    </checklist>
    <append field="vt_result" type="PLUGIN">virusTotal(file_hash)</append>
    <append field="action">quarantine</append>
</rule>
```

## Response Format

The plugin returns a `VirusTotalResult` object with the following structure:

```json
{
  "hash": "d41d8cd98f00b204e9800998ecf8427e",
  "md5": "d41d8cd98f00b204e9800998ecf8427e",
  "sha1": "da39a3ee5e6b4b0d3255bfef95601890afd80709",
  "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "detections": 2,
  "total_engines": 70,
  "malicious": 2,
  "suspicious": 0,
  "harmless": 65,
  "undetected": 3,
  "timeout": 0,
  "names": ["malware.exe", "trojan.bin"],
  "size": 1024,
  "type_tag": "executable",
  "first_seen": "2023-01-01T00:00:00Z",
  "last_seen": "2023-12-01T00:00:00Z",
  "engines": {
    "Kaspersky": "Trojan.Win32.Agent",
    "Symantec": "Trojan Horse"
  },
  "cached": false,
  "error": ""
}
```

### Field Descriptions

- **hash**: Input hash (normalized to lowercase)
- **md5/sha1/sha256**: All hash formats for the file
- **detections**: Total number of engines that detected the file as malicious or suspicious
- **total_engines**: Total number of engines that scanned the file
- **malicious**: Number of engines that flagged the file as malicious
- **suspicious**: Number of engines that flagged the file as suspicious
- **harmless**: Number of engines that found the file harmless
- **undetected**: Number of engines that didn't detect anything
- **timeout**: Number of engines that timed out
- **names**: Known filenames for this hash
- **size**: File size in bytes
- **type_tag**: File type (e.g., "executable", "document")
- **first_seen**: First submission date to VirusTotal
- **last_seen**: Last submission date to VirusTotal
- **engines**: Map of engine names to their detection results (only positive detections)
- **cached**: Whether the result came from cache
- **error**: Error message if query failed

## Error Handling

The plugin handles errors gracefully:

1. **Invalid hash format**: Returns result with error field
2. **API key not configured**: Returns result with error field
3. **Hash not found**: Returns result with error field
4. **API errors**: Returns result with error field
5. **Network errors**: Returns result with error field

## Caching

- **Cache Key**: `vt_cache:{hash}`
- **TTL**: 24 hours
- **Storage**: Redis
- **Deduplication**: Automatic based on hash

## Performance Considerations

- First query to VirusTotal takes ~1-2 seconds
- Cached queries return in <10ms
- Rate limiting handled by VirusTotal API
- Automatic retry on network errors

## Testing

Run the tests with:

```bash
cd src/local_plugin/virustotal
go test -v
```

## Supported Hash Formats

- **MD5**: 32 hexadecimal characters
- **SHA1**: 40 hexadecimal characters  
- **SHA256**: 64 hexadecimal characters

All hashes are automatically normalized to lowercase.

## Rate Limits

VirusTotal API has rate limits:
- **Free tier**: 4 requests per minute
- **Paid tier**: Higher limits available

The plugin automatically handles rate limiting through caching.

## Security Notes

- API key is read from environment variable only
- All communications use HTTPS
- No sensitive data is logged
- Cache keys are non-reversible

## Examples by Use Case

### 1. File Upload Scanning
```xml
<!-- Using environment variable -->
<rule id="upload_scan">
    <filter field="event_type">file_upload</filter>
    <append field="vt_scan" type="PLUGIN">virusTotal(file_sha256)</append>
</rule>

<!-- Using API key parameter -->
<rule id="upload_scan_with_key">
    <filter field="event_type">file_upload</filter>
    <append field="vt_scan" type="PLUGIN">virusTotal(file_sha256, "your_api_key")</append>
</rule>
```

### 2. Email Attachment Analysis
```xml
<rule id="email_attachment">
    <filter field="event_type">email_attachment</filter>
    <checklist>
        <node type="PLUGIN" field="attachment_hash">virusTotal(_$ORIDATA, "api_key").malicious > 0</node>
    </checklist>
    <append field="threat_detected">true</append>
</rule>
```

### 3. Process Execution Monitoring
```xml
<rule id="process_reputation">
    <filter field="event_type">process_start</filter>
    <append field="process_reputation" type="PLUGIN">virusTotal(process_hash, "api_key")</append>
</rule>
```

### 4. Threat Intelligence Enrichment
```xml
<rule id="intel_enrichment">
    <filter field="event_type">threat_indicator</filter>
    <checklist>
        <node type="EQU" field="indicator_type">hash</node>
    </checklist>
    <append field="vt_intel" type="PLUGIN">virusTotal(indicator_value, "your_api_key")</append>
</rule>
```

### 5. Multiple API Keys for Different Sources
```xml
<!-- Different rules can use different API keys -->
<rule id="internal_scan">
    <filter field="source">internal</filter>
    <append field="vt_result" type="PLUGIN">virusTotal(file_hash, "internal_api_key")</append>
</rule>

<rule id="external_scan">
    <filter field="source">external</filter>
    <append field="vt_result" type="PLUGIN">virusTotal(file_hash, "external_api_key")</append>
</rule>
``` 