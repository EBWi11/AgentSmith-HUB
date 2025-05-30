# AgentSmith-HUB

AgentSmith-HUB is a high-performance data processing and rules engine system designed for real-time data analysis and event detection. It provides a flexible and scalable architecture for processing streaming data from various sources, applying complex rules, and outputting results to multiple destinations.

## Features

- **Multi-source Input Support**
  - Kafka
  - Aliyun SLS (Simple Log Service)
  - Extensible input plugin system

- **Powerful Rules Engine**
  - Complex condition evaluation
  - Pattern matching with regex support
  - Threshold-based aggregation
  - Local and Redis-based caching
  - Plugin system for custom logic

- **Flexible Output Options**
  - Kafka
  - Elasticsearch
  - Aliyun SLS
  - Console printing
  - Extensible output plugin system

- **High Performance**
  - Channel-based message passing
  - Efficient caching mechanisms
  - Batch processing support
  - Concurrent rule evaluation

## Architecture

The system follows a modular architecture with three main components:

1. **Input Components**: Consume data from various sources
2. **Rules Engine**: Process and analyze data using configurable rules
3. **Output Components**: Send processed data to different destinations

Data flows through these components in a directed graph defined by project configuration.

## Configuration

### Project Configuration

Projects are defined in YAML files under the `config_demo/project` directory:

```yaml
id: "project_id"
name: "Project Name"
content: |
  INPUT.kafka1 -> RULESET.rules1
  RULESET.rules1 -> OUTPUT.es1
```

### Input Configuration

Input configurations are defined in YAML files under the `config_demo/input` directory:

```yaml
name: "Kafka Input"
type: "kafka"
kafka:
  brokers:
    - "localhost:9092"
  group: "consumer-group"
  topic: "input-topic"
  compression: "snappy"
  sasl:
    enable: true
    mechanism: "plain"
    username: "user"
    password: "pass"
```

### Rules Configuration

Rules are defined in XML files under the `config_demo/ruleset` directory:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<root name="Ruleset Name" type="DETECTION">
  <rule id="rule1" name="Rule Name" author="Author">
    <filter field="field.path">value</filter>
    <checklist condition="(a AND b) OR c">
      <node id="a" type="EQU" field="field1">value1</node>
      <node id="b" type="REGEX" field="field2">pattern</node>
      <node id="c" type="PLUGIN" field="field3">plugin_name(arg1, arg2)</node>
    </checklist>
    <threshold group_by="field1,field2" range="5m" count_type="SUM" count_field="field3" local_cache="true">10</threshold>
    <append type="PLUGIN" field_name="result">plugin_name(arg1, arg2)</append>
  </rule>
</root>
```

### Output Configuration

Output configurations are defined in YAML files under the `config_demo/output` directory:

```yaml
name: "Elasticsearch Output"
type: "elasticsearch"
elasticsearch:
  hosts:
    - "http://localhost:9200"
  index: "output-index"
  batch_size: 1000
  flush_dur: "5s"
```

## Usage

1. **Setup Configuration**
   - Create project configuration in `config_demo/project/`
   - Define input configurations in `config_demo/input/`
   - Create rules in `config_demo/ruleset/`
   - Configure outputs in `config_demo/output/`

2. **Start the System**
   ```go
   // Set configuration root
   project.SetConfigRoot("path/to/resource")
   
   // Create and start project
   p, err := project.NewProject("project.yaml")
   if err != nil {
       log.Fatal(err)
   }
   
   err = p.Start()
   if err != nil {
       log.Fatal(err)
   }
   ```

3. **Monitor and Manage**
   - Use `GetMetrics()` to monitor system performance
   - Check `GetLastError()` for error handling
   - Monitor `GetUptime()` for system health

## Rule Types

The rules engine supports various check types:

- **String Operations**
  - `EQU`: Equal to
  - `NEQ`: Not equal to
  - `INCL`: Contains
  - `NI`: Not contains
  - `START`: Starts with
  - `END`: Ends with
  - `NSTART`: Not starts with
  - `NEND`: Not ends with

- **Case-insensitive Operations**
  - `NCS_EQU`: Case-insensitive equal
  - `NCS_NEQ`: Case-insensitive not equal
  - `NCS_INCL`: Case-insensitive contains
  - `NCS_NI`: Case-insensitive not contains
  - `NCS_START`: Case-insensitive starts with
  - `NCS_END`: Case-insensitive ends with

- **Numeric Operations**
  - `MT`: More than
  - `LT`: Less than

- **Null Checks**
  - `ISNULL`: Is null/empty
  - `NOTNULL`: Is not null/empty

- **Pattern Matching**
  - `REGEX`: Regular expression match

- **Custom Logic**
  - `PLUGIN`: Custom plugin execution

## Performance Considerations

- Use appropriate batch sizes for outputs
- Configure local caching for frequently accessed data
- Use Redis for distributed caching when needed
- Monitor QPS metrics for performance tuning
- Adjust channel buffer sizes based on load

## Error Handling

The system provides comprehensive error handling:

- Input/Output connection errors
- Rule evaluation errors
- Plugin execution errors
- Configuration validation errors

All errors are logged and can be retrieved using `GetLastError()`.

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
