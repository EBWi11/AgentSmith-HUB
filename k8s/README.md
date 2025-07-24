# AgentSmith-HUB 原生 Kubernetes 部署

## 部署组件

1. **Redis** - 缓存和消息队列
2. **Leader** - 主节点，处理配置管理和集群协调，包含前端Web界面
3. **Follower** - 从节点，处理实际的数据处理任务

> **注意**: Leader 和 Follower 使用相同的 Docker 镜像 `ghcr.io/will/agentsmith-hub:latest`，通过环境变量 `MODE` 来区分角色：
> - Leader: `MODE=leader`，启动后端API和前端Web界面
> - Follower: `MODE=follower`，仅启动后端API，不包含前端

## 快速开始

### 前置条件

1. 确保你有可用的 Kubernetes 集群
2. 安装并配置 `kubectl`
3. 确保以下 Docker 镜像可用：
   - `ghcr.io/will/agentsmith-hub:latest` (Leader 和 Follower 共用同一个镜像，通过环境变量 MODE 区分)
   - `redis:7-alpine`
   
   > **注意**: 镜像可以通过 GitHub Actions 自动构建，详见 [Docker 镜像构建](#docker-镜像构建)

### 部署步骤

1. **运行部署脚本**：
   ```bash
   ./deploy.sh
   ```

2. **检查部署状态**：
   ```bash
   kubectl get all -n agentsmith-hub
   ```

3. **访问应用**：
   ```bash
   # 前端界面 (通过 Leader 服务)
   kubectl port-forward svc/agentsmith-hub-leader 8080:80 -n agentsmith-hub
   
   # API 接口
   kubectl port-forward svc/agentsmith-hub-leader 8081:8080 -n agentsmith-hub
   ```

### 清理部署

```bash
./cleanup.sh
```

## 配置说明

### 环境变量

- `MODE` - 运行模式 (leader/follower)
- `REDIS_HOST` - Redis 服务地址
- `REDIS_PORT` - Redis 端口 (6379)
- `REDIS_PASSWORD` - Redis 密码
- `NODE_ID` - 节点标识
- `LOG_LEVEL` - 日志级别
- `CONFIG_ROOT` - 配置文件路径
- `AGENTSMITH_TOKEN` - 认证令牌

### 资源配置

- **Leader**: 1Gi 内存，500m CPU (请求) / 2Gi 内存，1000m CPU (限制)
- **Follower**: 1Gi 内存，500m CPU (请求) / 2Gi 内存，1000m CPU (限制)
- **Redis**: 256Mi 内存，250m CPU (请求) / 512Mi 内存，500m CPU (限制)

## 自定义配置

### 1. 更新配置文件

编辑 `k8s-deployment.yaml` 中的 ConfigMap 部分，添加你的实际配置文件：

```yaml
data:
  config.yaml: |
    # 你的 AgentSmith-HUB 配置
    # 复制 config/config.yaml 的内容
```

### 2. 修改镜像版本

在 deployment 配置中更新镜像标签（Leader 和 Follower 使用相同镜像）：

```yaml
# Leader 和 Follower 都使用这个镜像
image: your-registry/agentsmith-hub:your-version
```

### 3. 调整资源配置

根据需要修改 resources 部分：

```yaml
resources:
  requests:
    memory: "2Gi"
    cpu: "1000m"
  limits:
    memory: "4Gi"
    cpu: "2000m"
```

### 4. 配置持久化存储

如果需要持久化存储，将 `emptyDir` 替换为 `PersistentVolumeClaim`：

```yaml
volumes:
- name: redis-data
  persistentVolumeClaim:
    claimName: redis-pvc
```

## 监控和日志

### 查看日志

```bash
# Leader 日志
kubectl logs -f deployment/agentsmith-hub-leader -n agentsmith-hub

# Follower 日志
kubectl logs -f deployment/agentsmith-hub-follower -n agentsmith-hub

# Redis 日志
kubectl logs -f deployment/agentsmith-hub-redis -n agentsmith-hub
```

### 查看状态

```bash
# 查看所有资源
kubectl get all -n agentsmith-hub

# 查看 Pod 状态
kubectl get pods -n agentsmith-hub

# 查看服务
kubectl get services -n agentsmith-hub

# 查看配置
kubectl get configmaps -n agentsmith-hub
kubectl get secrets -n agentsmith-hub
```

## 故障排除

### 常见问题

1. **Pod 启动失败**
   - 检查镜像是否存在（Leader 和 Follower 使用相同镜像 `ghcr.io/will/agentsmith-hub:latest`）
   - 查看 Pod 日志：`kubectl logs <pod-name> -n agentsmith-hub`
   - 检查资源配置是否足够

2. **服务无法访问**
   - 检查 Service 配置
   - 确认 Pod 标签匹配
   - 验证端口配置

3. **配置加载失败**
   - 检查 ConfigMap 内容
   - 确认挂载路径正确
   - 查看应用日志

### 调试命令

```bash
# 进入 Pod 调试
kubectl exec -it <pod-name> -n agentsmith-hub -- /bin/sh

# 查看 Pod 详细信息
kubectl describe pod <pod-name> -n agentsmith-hub

# 查看事件
kubectl get events -n agentsmith-hub --sort-by='.lastTimestamp'
```

## 注意事项

1. 默认使用 `emptyDir` 存储，重启后数据会丢失
2. 生产环境建议配置持久化存储
3. 默认 token 为 `9ef0c170-069e-44dd-a406-2d85eca0a0b2`，生产环境请修改
4. 所有组件都在 `agentsmith-hub` 命名空间中
5. 需要确保集群有足够的资源

## Docker 镜像构建

### GitHub Actions 自动构建

项目配置了 GitHub Actions 工作流来自动构建和推送 Docker 镜像到 GitHub Container Registry (GHCR)。

#### 触发条件

- 推送到 `main` 或 `develop` 分支
- 创建版本标签 (如 `v1.0.0`)
- 创建 Pull Request

#### 构建的镜像

- **AgentSmith-HUB**: `ghcr.io/will/agentsmith-hub:latest` (统一镜像，包含前端和后端)

#### 镜像标签

- `latest`: 最新版本
- `{version}`: 版本标签 (如 `v1.0.0`)
- `{arch}-latest`: 架构特定最新版本 (如 `amd64-latest`)
- `{arch}-{version}`: 架构特定版本 (如 `amd64-v1.0.0`)

#### 支持的架构

- **AMD64**: Intel/AMD x86_64 处理器
- **ARM64**: ARM 64位处理器 (Apple Silicon, ARM 服务器)

#### 本地构建

如果需要本地构建镜像：

```bash
# 构建统一镜像（包含前端和后端）
docker build -t agentsmith-hub:latest .
```

### 使用自定义镜像

在 `agentsmith-hub-deployment.yaml` 中修改镜像地址：

```yaml
# 使用 GitHub Container Registry
image: ghcr.io/will/agentsmith-hub:latest

# 使用本地镜像
image: agentsmith-hub:latest

# 使用特定版本
image: ghcr.io/will/agentsmith-hub:v1.0.0
```

## 支持

如果遇到问题，请：

1. 查看 Pod 日志
2. 检查 Kubernetes 事件
3. 验证配置文件格式
4. 确认网络连接正常
5. 检查镜像是否存在且可访问 