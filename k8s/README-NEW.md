# AgentSmith-HUB 新Kubernetes部署指南

## 概述

本指南描述了如何使用新的统一Docker镜像部署AgentSmith-HUB，该镜像同时支持leader和follower模式，并包含前后端组件。

## 主要改进

1. **统一镜像**: 前后端合并到一个镜像中
2. **启动模式**: 通过启动脚本区分leader和follower模式
3. **配置持久化**: leader配置使用PVC持久化存储
4. **简化部署**: 减少镜像数量和部署复杂度

## 部署步骤

### 1. 更新镜像地址

编辑 `k8s-deployment-new.yaml` 文件，将镜像地址替换为你的实际镜像地址：

```yaml
image: ghcr.io/your-username/agentsmith-hub:latest
```

### 2. 配置持久化存储

确保你的Kubernetes集群支持动态存储供应，或者修改PVC配置：

```yaml
storageClassName: standard  # 根据你的集群调整
```

### 3. 部署到Kubernetes

```bash
# 创建命名空间和应用
kubectl apply -f k8s-deployment-new.yaml

# 检查部署状态
kubectl get pods -n agentsmith-hub

# 查看日志
kubectl logs -f deployment/agentsmith-hub-leader -n agentsmith-hub
```

## 服务访问

### 内部访问
- **Leader**: `http://agentsmith-hub-leader.agentsmith-hub.svc.cluster.local`
- **Follower**: `http://agentsmith-hub-follower.agentsmith-hub.svc.cluster.local`

### 外部访问
如果配置了Ingress：
- **Leader**: `http://agentsmith-hub.local`
- **Follower**: `http://agentsmith-follower.local`

## 配置管理

### Leader配置持久化
Leader使用PVC持久化存储配置，路径为 `/opt/config`。你可以通过以下方式更新配置：

```bash
# 获取leader pod名称
POD_NAME=$(kubectl get pods -n agentsmith-hub -l app=agentsmith-hub-leader -o jsonpath='{.items[0].metadata.name}')

# 进入pod
kubectl exec -it $POD_NAME -n agentsmith-hub -- /bin/bash

# 编辑配置文件
vi /opt/config/config.yaml

# 重启pod应用配置
kubectl delete pod $POD_NAME -n agentsmith-hub
```

### 配置备份
```bash
# 备份leader配置
kubectl exec -n agentsmith-hub deployment/agentsmith-hub-leader -- tar -czf /tmp/config-backup.tar.gz /opt/config/
kubectl cp agentsmith-hub/$POD_NAME:/tmp/config-backup.tar.gz ./config-backup.tar.gz
```

## 扩展部署

### 增加Follower副本
```bash
kubectl scale deployment agentsmith-hub-follower --replicas=5 -n agentsmith-hub
```

### 更新镜像
```bash
kubectl set image deployment/agentsmith-hub-leader agentsmith-hub=ghcr.io/your-username/agentsmith-hub:v1.2.3 -n agentsmith-hub
kubectl set image deployment/agentsmith-hub-follower agentsmith-hub=ghcr.io/your-username/agentsmith-hub:v1.2.3 -n agentsmith-hub
```

## 监控和日志

### 查看服务状态
```bash
kubectl get all -n agentsmith-hub
```

### 查看日志
```bash
# Leader日志
kubectl logs -f deployment/agentsmith-hub-leader -n agentsmith-hub

# Follower日志
kubectl logs -f deployment/agentsmith-hub-follower -n agentsmith-hub
```

### 资源监控
```bash
kubectl top pods -n agentsmith-hub
```

## 故障排除

### Pod启动失败
1. 检查镜像拉取权限
2. 确认PVC绑定状态
3. 查看详细事件信息

### 配置不生效
1. 确认配置文件路径正确
2. 检查leader配置持久化
3. 重启相关pod

### 网络连接问题
1. 检查服务发现配置
2. 确认Redis连接
3. 验证网络策略