# Hooky Registry

🐳 **Hooky Registry** 是一个集成了 Docker Registry 和 Webhook 接收器的 Go 应用程序，提供完整的私有镜像仓库解决方案。

## ✨ 特性

- **集成化部署**: 单个 Docker 镜像包含 Registry 和 Webhook 接收器
- **自动化更新**: 监听 Registry 推送事件，自动更新使用该镜像的容器
- **多种更新策略**: 支持`recreate`（重建）和`restart`（重启）两种更新模式
- **健康检查**: 内置健康检查端点，监控服务状态
- **多架构支持**: 支持 AMD64 和 ARM64 架构
- **生产就绪**: 基于官方 Registry 镜像，使用 Supervisor 管理多进程

## 🚀 快速开始

### 使用 Docker Compose（推荐）

```bash
# 克隆项目
git clone https://github.com/hnrobert/hooky-registry.git
cd hooky-registry

# 启动服务
docker-compose up -d
```

### 使用预构建镜像

```bash
# 拉取最新镜像
docker pull ghcr.io/hnrobert/hooky-registry:latest

# 运行容器
docker run -d \
  --name hooky-registry \
  -p 5000:5000 \
  -p 5001:5001 \
  -v registry-data:/var/lib/registry \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e UPDATE_STRATEGY=recreate \
  ghcr.io/hnrobert/hooky-registry:latest
```

## 📖 配置说明

### 环境变量

| 变量名            | 默认值           | 说明                              |
| ----------------- | ---------------- | --------------------------------- |
| `REGISTRY`        | `127.0.0.1:5000` | Registry 服务地址                 |
| `WEBHOOK_PORT`    | `5001`           | Webhook 接收器端口                |
| `UPDATE_STRATEGY` | `recreate`       | 更新策略：`recreate` 或 `restart` |

### 端口映射

- **5000**: Docker Registry HTTP API
- **5001**: Webhook 接收器 API

### 数据持久化

- `/var/lib/registry`: Registry 数据存储目录
- `/var/run/docker.sock`: Docker 守护进程套接字（用于容器管理）

## 🔧 Registry 配置

项目包含一个预配置的`config.yml`文件，启用了 Webhook 通知功能：

```yaml
version: 0.1
log:
  fields:
    service: registry
storage:
  cache:
    blobdescriptor: inmemory
  filesystem:
    rootdirectory: /var/lib/registry
http:
  addr: :5000
  headers:
    X-Content-Type-Options: [nosniff]
health:
  storagedriver:
    enabled: true
    interval: 10s
    threshold: 3
notifications:
  endpoints:
    - name: hooky-webhook
      url: http://127.0.0.1:5001/webhook
      timeout: 10s
      threshold: 3
      backoff: 1s
      headers:
        Authorization: [Bearer your-secret-token]
```

## 📡 API 端点

### Registry (端口 5000)

- `GET /v2/`: Registry API 健康检查
- `GET /v2/_catalog`: 列出所有仓库
- `GET /v2/{name}/tags/list`: 列出镜像标签

### Webhook 接收器 (端口 5001)

- `GET /`: 服务信息
- `GET /health`: 健康检查
- `POST /webhook`: 接收 Registry Webhook 事件

### 健康检查示例

```bash
# Registry健康检查
curl http://localhost:5000/v2/

# Webhook接收器健康检查
curl http://localhost:5001/health
```

## 🔄 更新策略

### Recreate 模式（默认）

- 停止并删除旧容器
- 使用新镜像创建容器
- 适用于无状态应用

### Restart 模式

- 简单重启现有容器
- 容器会拉取新镜像（如果策略配置为 always）
- 适用于开发环境

## 🛠️ 开发

### 本地构建

```bash
# 克隆项目
git clone https://github.com/hnrobert/hooky-registry.git
cd hooky-registry

# 构建Go应用
go mod download
go build -o webhook-receiver main.go

# 构建Docker镜像
docker build -t hooky-registry:local .

# 运行测试
docker-compose up -d
```

### 项目结构

```text
hooky-registry/
├── main.go              # Webhook接收器主程序
├── go.mod               # Go模块定义
├── go.sum               # Go依赖校验
├── Dockerfile           # 多阶段构建配置
├── supervisord.conf     # Supervisor进程管理配置
├── docker-compose.yaml  # Docker Compose配置
├── config.yml           # Registry配置文件
└── .github/workflows/   # CI/CD配置
    └── build.yml        # 自动构建和发布
```

## 🚢 部署

### GitHub Actions 自动构建

项目配置了自动 CI/CD 流水线：

- **main 分支**: 构建并推送 `latest` 标签
- **develop 分支**: 构建并推送 `develop` 标签
- **PR**: 构建测试，不推送

### 手动部署

```bash
# 部署到生产环境
docker pull ghcr.io/hnrobert/hooky-registry:latest
docker stop hooky-registry || true
docker rm hooky-registry || true
docker run -d \
  --name hooky-registry \
  --restart unless-stopped \
  -p 5000:5000 \
  -p 5001:5001 \
  -v registry-data:/var/lib/registry \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/hnrobert/hooky-registry:latest
```

## 🔍 监控和日志

### 查看日志

```bash
# 查看所有日志
docker logs hooky-registry

# 实时跟踪日志
docker logs -f hooky-registry

# 查看特定服务日志
docker exec hooky-registry tail -f /var/log/supervisor/registry.log
docker exec hooky-registry tail -f /var/log/supervisor/webhook.log
```

### 监控服务状态

```bash
# 检查容器状态
docker ps | grep hooky-registry

# 检查进程状态
docker exec hooky-registry supervisorctl status

# 健康检查
curl -f http://localhost:5000/v2/ && curl -f http://localhost:5001/health
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 开启 Pull Request

## 📄 许可证

本项目基于 MIT 许可证开源。详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- [Docker Registry](https://docs.docker.com/registry/) - 官方 Docker 镜像仓库
- [Gorilla Mux](https://github.com/gorilla/mux) - Go HTTP 路由器
- [Supervisor](http://supervisord.org/) - 进程管理工具

---

**⭐ 如果这个项目对你有帮助，请给个 Star 支持一下！**
