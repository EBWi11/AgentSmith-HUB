# Docker 构建指南

## 概述

AgentSmith-HUB 现在使用预编译的二进制文件进行Docker镜像构建，避免了在容器内编译的问题。这种方法有以下优势：

- ✅ 避免交叉编译问题
- ✅ 更快的镜像构建速度
- ✅ 更小的镜像体积
- ✅ 更可靠的构建过程

## 本地构建

### 1. 先构建二进制文件

```bash
# 构建前端
cd web
npm ci
npm run build
cd ..

# 构建后端二进制文件
# 对于 AMD64
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=amd64
LIB_PATH="$(pwd)/lib/linux/amd64"
export CGO_LDFLAGS="-L${LIB_PATH} -lrure -Wl,-rpath,${LIB_PATH}"
export LD_LIBRARY_PATH="${LIB_PATH}:$LD_LIBRARY_PATH"
cd src
go build -ldflags "-s -w" -o ../build/agentsmith-hub-amd64 .
cd ..

# 对于 ARM64
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=arm64
export CC=aarch64-linux-gnu-gcc
LIB_PATH="$(pwd)/lib/linux/arm64"
export CGO_LDFLAGS="-L${LIB_PATH} -lrure -Wl,-rpath,${LIB_PATH}"
export LD_LIBRARY_PATH="${LIB_PATH}:$LD_LIBRARY_PATH"
cd src
go build -ldflags "-s -w" -o ../build/agentsmith-hub-arm64 .
cd ..
```

### 2. 构建Docker镜像

```bash
# 构建多架构镜像
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 -t agentsmith-hub:latest --push .

# 或者构建单架构镜像
docker build -t agentsmith-hub:amd64 --build-arg TARGETARCH=amd64 .
docker build -t agentsmith-hub:arm64 --build-arg TARGETARCH=arm64 .
```

## GitHub Actions 自动构建

GitHub Actions 工作流会自动：

1. 构建 AMD64 和 ARM64 二进制文件
2. 创建部署压缩包
3. 构建并推送多架构Docker镜像到：
   - Docker Hub: `yourusername/agentsmith-hub`
   - GitHub Container Registry: `ghcr.io/yourusername/agentsmith-hub`

### 所需密钥

在GitHub仓库设置中添加以下密钥：

- `DOCKER_USERNAME`: Docker Hub 用户名
- `DOCKER_PASSWORD`: Docker Hub 密码或访问令牌

## 使用镜像

### 从 Docker Hub 拉取

```bash
docker pull yourusername/agentsmith-hub:latest
```

### 从 GitHub Container Registry 拉取

```bash
docker pull ghcr.io/yourusername/agentsmith-hub:latest
```

### 运行容器

```bash
docker run -d \
  --name agentsmith-hub \
  -p 8080:8080 \
  -v $(pwd)/config:/opt/agentsmith-hub/config \
  -v $(pwd)/mcp_config:/opt/agentsmith-hub/mcp_config \
  yourusername/agentsmith-hub:latest
```

## 统一镜像使用方式

### 运行 Leader 模式（包含前端）
```bash
docker run -d \
  --name agentsmith-hub-leader \
  -p 8080:8080 \
  -p 80:80 \
  -e MODE=leader \
  -e NODE_ID=leader-01 \
  -e LOG_LEVEL=info \
  -v /path/to/config:/opt/config \
  ghcr.io/your-username/agentsmith-hub:latest
```

### 运行 Follower 模式
```bash
docker run -d \
  --name agentsmith-hub-follower \
  -p 8081:8080 \
  -e MODE=follower \
  -e NODE_ID=follower-01 \
  -e LEADER_ADDR=http://leader-host:8080 \
  -e LOG_LEVEL=info \
  ghcr.io/your-username/agentsmith-hub:latest
```

### Kubernetes 部署
```bash
# 使用新的统一部署文件
kubectl apply -f k8s/k8s-deployment-new.yaml
```

## 配置持久化

### Leader 配置持久化
Leader 模式支持配置持久化：

```bash
# Docker 方式挂载配置目录
docker run -d \
  --name agentsmith-hub-leader \
  -v $(pwd)/config:/opt/config \
  -v agentsmith-config:/opt/config \
  ghcr.io/your-username/agentsmith-hub:latest

# 创建 Docker 卷
docker volume create agentsmith-config
docker run -d \
  --name agentsmith-hub-leader \
  -v agentsmith-config:/opt/config \
  ghcr.io/your-username/agentsmith-hub:latest
```

## 故障排除

### 构建问题
1. **架构不匹配**: 确保目标架构与构建主机匹配
2. **库文件缺失**: 检查 `lib/linux/${TARGETARCH}/` 目录内容
3. **权限问题**: 确保二进制文件有执行权限

### 运行时问题
1. **模式切换**: 使用 `MODE` 环境变量切换 leader/follower
2. **端口冲突**: 8080和80端口被占用时修改端口映射
3. **配置错误**: 检查配置文件路径和内容
4. **权限错误**: 确认容器以正确用户身份运行

### 跨平台构建
使用 `docker buildx` 进行跨平台构建：
```bash
docker buildx build --platform linux/amd64,linux/arm64 -t agentsmith-hub:latest .
```

### 调试模式
```bash
# 进入容器调试
docker exec -it agentsmith-hub-leader /bin/bash

# 查看日志
docker logs agentsmith-hub-leader

# 检查配置文件
docker exec agentsmith-hub-leader cat /opt/config/config.yaml
```

## Docker 镜像架构

新的 Docker 镜像采用统一架构：
- **单一镜像** 同时支持 leader 和 follower 模式
- **前后端合并** 在一个容器中运行
- **模式特定启动脚本** 用于区分 leader/follower 行为

### 架构支持
- **AMD64** (x86_64)
- **ARM64** (aarch64)

### 镜像组件
- 预编译二进制文件 (`agentsmith-hub`)
- 系统库 (`librure.so`)
- Web 前端 (`web/dist/`)
- 配置文件 (`config/`, `mcp_config/`)
- Nginx Web 服务器用于前端服务
- 模式特定启动脚本 (`leader-start.sh`, `follower-start.sh`)
- 健康检查端点
- 非 root 用户执行

### 架构支持对比

| 架构 | 支持状态 | 备注 |
|------|----------|------|
| AMD64 | ✅ 完全支持 | x86_64, Intel/AMD |
| ARM64 | ✅ 完全支持 | Apple Silicon, ARM服务器 |

## 版本标签

自动构建会生成以下标签：

- `latest`: 主分支最新版本
- `v1.0.0`: 具体版本号
- `v1.0`: 主版本.次版本
- `main`: 主分支构建
- `develop`: 开发分支构建

## 性能优化

- 镜像基于 Alpine Linux，体积小
- 使用非root用户运行，安全性高
- 包含健康检查功能
- 支持环境变量配置