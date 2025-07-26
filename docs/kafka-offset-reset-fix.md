# Kafka Consumer Offset Reset Fix

## 问题描述

在之前的版本中，AgentSmith-HUB 的 Kafka 输入组件存在一个重要问题：当 Kafka 消费者重启时，如果没有已提交的 offset，消费者会使用默认的 `latest` 策略，这意味着只会消费重启后的新消息，而不会从上次停止的位置继续消费。这会导致消息丢失。

## 症状

- 有 lag 但是不消费数据
- 重新写入数据后才开始消费
- 消费者重启后丢失消息

## 解决方案

### 1. 后端修改

#### a. 添加 `offset_reset` 配置选项

在 `src/input/input.go` 中的 `KafkaInputConfig` 结构体添加了新字段：

```go
type KafkaInputConfig struct {
    Brokers       []string                    `yaml:"brokers"`
    Group         string                      `yaml:"group"`
    Topic         string                      `yaml:"topic"`
    Compression   common.KafkaCompressionType `yaml:"compression,omitempty"`
    SASL          *common.KafkaSASLConfig     `yaml:"sasl,omitempty"`
    TLS           *common.KafkaTLSConfig      `yaml:"tls,omitempty"`
    OffsetReset   string                      `yaml:"offset_reset,omitempty"` // earliest, latest, or none
}
```

#### b. 修改 Kafka 消费者创建逻辑

在 `src/common/kafka.go` 中的 `NewKafkaConsumer` 函数添加了 `offset_reset` 参数处理：

```go
func NewKafkaConsumer(brokers []string, group, topic string, compression KafkaCompressionType, saslCfg *KafkaSASLConfig, tlsCfg *KafkaTLSConfig, offsetReset string, msgChan chan map[string]interface{}) (*KafkaConsumer, error) {
    // 根据配置设置 offset reset 策略
    switch offsetReset {
    case "latest":
        opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
    case "none":
        // 不设置任何 reset offset - 如果没有已提交的 offset 将会失败
    case "earliest", "":
        // 默认为 earliest 以确保不丢失消息
        opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
    default:
        return nil, fmt.Errorf("invalid offset_reset value: %s (valid values: earliest, latest, none)", offsetReset)
    }
}
```

### 2. 前端修改

#### a. 更新模板生成器

在 `web/src/utils/templateGenerator.js` 中更新了输入组件模板：

```yaml
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "consumer-group"
  topic: "test-topic"
  compression: "none"
  offset_reset: earliest  # 新增字段
```

#### b. 更新 Monaco 编辑器自动补全

在 `web/src/components/MonacoEditor.vue` 中：

1. 添加了 `offset_reset` 键的自动补全
2. 添加了 `offset_reset` 值的自动补全（earliest/latest/none）

### 3. 配置选项说明

- **`earliest`** (默认，推荐): 当没有已提交的 offset 时，从 topic 的最早消息开始消费。这确保了数据完整性。
- **`latest`**: 当没有已提交的 offset 时，从 topic 的最新消息开始消费。适用于只关心实时数据的场景。
- **`none`**: 如果没有已提交的 offset 则失败。适用于严格的消费要求。

## 使用方法

### 新配置文件

```yaml
type: kafka
kafka:
  brokers:
    - "your-kafka-broker:9092"
  group: "your-consumer-group"
  topic: "your-topic"
  offset_reset: earliest  # 推荐设置
```

### 现有配置文件

如果现有配置文件没有 `offset_reset` 字段，系统会默认使用 `earliest` 策略。

## 向后兼容性

- 现有配置文件无需修改，会自动使用安全的默认值（`earliest`）
- API 接口保持兼容
- 不会影响已经运行的消费者的 offset 提交机制

## 测试建议

1. 创建新的输入组件配置
2. 启动消费者并让其消费一些消息
3. 停止消费者
4. 重启消费者，确认它从正确的位置继续消费

## 注意事项

- 此修复主要影响首次启动或重新分配分区时的行为
- 如果消费者组已有已提交的 offset，`offset_reset` 设置不会生效
- 建议在生产环境中使用 `earliest` 以避免消息丢失
