# AgentSmith-HUB Performance Testing Report

## Executive Summary

This document presents the performance testing results for AgentSmith-HUB, a high-performance rules engine for real-time data processing. The tests were conducted to evaluate the system's capability to handle high-throughput message processing with complex rule evaluation.

## Test Environment

### System Configuration
- **OS**: Debian 12.10 (Linux) VM
- **CPU**: 2 vCPUs (allocated from M3 cores)
- **Memory**: 4GB RAM
- **Storage**: Virtual disk on SSD

## Test Data

### Sample Message Structure
```json
{
    "name": "John Doe",
    "age": 30,
    "city": "New York",
    "isStudent": false,
    "ip": "192.168.76.135",
    "port": 9092,
    "protocol": "kafka",
    "topic": "test",
    "data": {
        "sub_01": "# Initialize Kafka producer - Updated to match Hub configuration",
        "sub_02": "# value_serializer=lambda x: json.dumps(x).encode('utf-8'), 78d9j1mdk1adf_67"
    },
    "partition": 0,
    "offset": 0,
    "courses": [
        "Math",
        "Science"
    ]
}
```

### Test Project Configuration

#### Project Flow
```
INPUT.test -> RULESET.test
RULESET.test -> RULESET.test_whitelist
```

#### Primary Ruleset (test)
```xml
<root author="name">
    <rule id="rule_01">
        <check type="EQU" field="name">Doe</check>
        <check type="MT" field="age">30</check>
    </rule>

    <rule id="rule_02">
        <checklist condition="a and b or c">
            <check id="a" type="INCL" field="name">John</check>
            <check id="b" type="LT" field="age">50</check>
            <check id="c" type="REGEX" field="data.sub_01">s+Updated[a-z]+</check>
        </checklist>
    </rule>

    <rule id="rule_03">
        <check type="REGEX" field="data.sub_02">^\d{2}[a-z]\d[a-z]\d[a-z]{2}\d[a-z]{3}_\d{2}[\s\S]*$</check>
    </rule>

    <rule id="rule_04">
        <check type="PLUGIN">isPrivateIP(ip)</check>
    </rule>

    <rule id="rule_05">
        <check type="EQU" field="protocol">kafka</check>
        <check type="START" field="topic">test</check>
        <append field="test_field">test_value</append>
        <append type="PLUGIN" field="now">now()</append>
        <check type="INCL" field="test_field">value</check>
    </rule>

    <rule id="rule_06">
        <check type="NCS_INCL" field="city">york</check>
        <check type="NCS_INCL" field="city">new</check>
        <append field="base64">base64Encode(data.sub_01)</append>
        <append field="unbase64">base64Decode(base64)</append>
        <check type="NOTNULL" field="unbase64"></check>
        <del>base64</del>
    </rule>

    <rule id="rule_07">
        <check type="NCS_INCL" field="city">york</check>
        <check type="NCS_INCL" field="city">new</check>
        <check type="EQU" field="port">9092</check>
        <check type="REGEX" field="isStudent">.*alse</check>
    </rule>
</root>
```

#### Whitelist Ruleset (test_whitelist)
```xml
<root author="will" type="WHITELIST">
    <rule id="rule_01" name="name">
        <check type="INCL" field="city">new</check>
        <check type="INCL" field="city">york</check>
    </rule>

    <rule id="rule_02" name="name">
        <check type="INCL" field="data.sub_01" logic="OR" delimiter="|">Python|world|people|China|Java|one|two|three|innovation|language|cloud|futudre|digital|learn|create|sun|moon|star|river|mountain|forest|ocean|city|village|school|book|pen|music|art|love|peace|hope|dream|success|failure|time|space|energy|light|dark|science|technology|history|culture|travel|adventure|food|water|fire|earth|air|animal|plant|tree|flower|bird|fish|dog|cat|friend|family|teacher|student|idea|thought|question|answer|problem|solution|change|growth|beginning|end|today|tomorrow|yesterday|moment|memory|future|past|secret|truth|lie|story|journey|path|road|door|window|house|room|garden|field|sky|weather|rain|snow|wind|hot|cold|warm|cool|color|sound|silence|voice|laugh|smile|producer</check>
    </rule>
</root>
```

## Performance Test Results

### Throughput Performance
- **Average QPS**: 40,000 messages per second
- **Average CPU Utilization**: 200%
- **Average Memory Usage**: 350MB
- **Latency**: Sub-millisecond processing time per message (with minor variations) 