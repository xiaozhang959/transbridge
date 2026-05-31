# TransBridge 部署指南

本文档提供了 TransBridge 的多种部署方式，包括直接运行、Docker 容器、系统服务等。

## 目录

- [环境要求](#环境要求)
- [直接运行](#直接运行)
- [系统服务部署](#系统服务部署)
- [Docker 部署](#docker-部署)
- [Kubernetes 部署](#kubernetes-部署)
- [反向代理配置](#反向代理配置)
- [性能优化](#性能优化)
- [监控设置](#监控设置)
- [安全建议](#安全建议)

## 环境要求

- 支持的操作系统：Linux, macOS, Windows
- 内存建议：至少 512MB
- 硬盘空间：至少 100MB
- 如需 Redis 缓存：Redis 服务器

## 直接运行

1. 下载最新的二进制文件或从源码编译：

```bash
# 从源码编译
git clone https://github.com/fruitbars/transbridge.git
cd transbridge
./build.sh

# 或直接下载编译好的二进制文件
```

2. 创建配置文件 `config.yml`：

```yaml
server:
  port: 8080

providers:
  - provider: "openai"
    api_url: "https://api.openai.com/v1/chat/completions"
    api_key: "your-api-key"
    timeout: 30
    is_default: true
    models:
      - name: "gpt-3.5-turbo"
        weight: 10
        max_tokens: 2000
        temperature: 0.3

cache:
  enabled: true
  types: ["memory"]
  memory:
    ttl:
      value: "1h"
    max_size: 10000

transapi:
  tokens:
    - "your-api-key"
```

3. 运行服务：

```bash
./transbridge -config config.yml
```

## 系统服务部署

### Linux (systemd)

1. 创建服务文件：

```bash
sudo vim /etc/systemd/system/transbridge.service
```

2. 添加以下内容：

```ini
[Unit]
Description=TransBridge Translation Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/transbridge
ExecStart=/opt/transbridge/transbridge -config /opt/transbridge/config.yml
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

3. 创建目录并移动文件：

```bash
sudo mkdir -p /opt/transbridge
sudo cp transbridge /opt/transbridge/
sudo cp config.yml /opt/transbridge/
```

4. 启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable transbridge
sudo systemctl start transbridge
```

5. 检查状态：

```bash
sudo systemctl status transbridge
```

### 或使用提供的安装脚本

```bash
chmod +x install-transbridge.sh
sudo ./install-transbridge.sh
```

## 🐳 Docker 部署

### 使用 Docker Compose（推荐）

项目提供了完整的 Docker Compose 配置，可以快速部署 TransBridge 服务和 Redis 缓存：

1. 确保已安装 [Docker](https://docs.docker.com/get-docker/) 和 [Docker Compose](https://docs.docker.com/compose/install/)

2. 创建 `.env` 文件（或使用项目提供的示例）
```bash
cp .env.example .env
# 根据需要修改 .env 文件中的配置
```

3. 启动服务
```bash
docker-compose up -d
```

4. 查看日志
```bash
docker-compose logs -f
```

5. 停止服务
```bash
docker-compose down
```

Docker Compose 配置提供了以下功能：
- 自动构建和启动 TransBridge 服务
- 可选的 Redis 缓存服务
- 配置文件和日志目录挂载
- 健康检查和自动重启
- 灵活的环境变量配置

## ▲ Vercel 部署

[![Deploy with Vercel](https://vercel.com/button)][vercel-deploy]

项目已内置 Vercel Go HTTP Server 入口：

```text
main.go
vercel.json
```

部署步骤：

1. 将仓库导入 Vercel
2. 在 Environment Variables 中配置 `config.yml` 使用到的变量
3. 部署后访问：
   - `https://your-domain.vercel.app/translate`
   - `https://your-domain.vercel.app/immersivel`
   - `https://your-domain.vercel.app/v1/chat/completions`
   - `https://your-domain.vercel.app/v1/models`
   - `https://your-domain.vercel.app/health`

一键部署按钮会自动列出 Vercel 需要的环境变量，并为非敏感配置预填默认值。
`vercel.json` 也内置了非敏感默认值，敏感项仍需要手动填写。
最少需要手动填写：

```env
PROVIDER_1_API_KEY=sk-your-api-key
TRANSAPI_TOKENS=["tr-demo-token"]
```

也可以参考仓库根目录的 `.env.vercel.example` 手动配置。

Telegram Bot 在 Vercel 上必须使用 webhook：

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=https://your-domain.vercel.app/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

建议额外配置：

```env
TELEGRAM_ENABLED=true
TELEGRAM_BOT_TOKEN=1234567890:your_bot_token
TELEGRAM_BOT_USERNAME=your_bot_username
TELEGRAM_WEBHOOK_SECRET=change-me-to-a-random-secret
```

注意事项：

- Vercel 环境不会运行 `getUpdates` 长轮询。
- 文件日志在 Vercel 中会自动禁用，请使用 Vercel 平台日志查看运行记录。
- 如果只启用内存缓存，冷启动或实例回收后缓存和 Telegram 会话状态会丢失。
- 如需跨冷启动保存 `/auto` 和“上一条消息”，建议配置 Redis，并按需设置
  `REDIS_TLS=true`。

### 使用 Docker 构建和运行

也可以直接使用 Docker 命令构建和运行：

```bash
# 构建镜像
docker build -t transbridge .

# 运行容器
docker run -d -p 8080:8080 -v $(pwd)/config.yml:/app/config.yml --name transbridge transbridge

# 指定版本信息构建
docker build \
  --build-arg BUILD_VERSION=1.0.0 \
  --build-arg BUILD_DATE=$(date -u +'%Y-%m-%d_%H:%M:%S') \
  --build-arg COMMIT_HASH=$(git rev-parse --short HEAD) \
  -t transbridge:1.0.0 .
```

## Kubernetes 部署

1. 创建配置文件 `transbridge-config.yaml`：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: transbridge-config
data:
  config.yml: |
    server:
      port: 8080
    providers:
      - provider: "openai"
        api_url: "https://api.openai.com/v1/chat/completions"
        api_key: "${OPENAI_API_KEY}"
        timeout: 30
        is_default: true
        models:
          - name: "gpt-3.5-turbo"
            weight: 10
            max_tokens: 2000
            temperature: 0.3
    cache:
      enabled: true
      types: ["memory"]
      memory:
        ttl:
          value: "1h"
        max_size: 10000
    auth:
      tokens:
        - "${AUTH_TOKEN}"
```

2. 创建部署文件 `transbridge-deployment.yaml`：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: transbridge
  labels:
    app: transbridge
spec:
  replicas: 2
  selector:
    matchLabels:
      app: transbridge
  template:
    metadata:
      labels:
        app: transbridge
    spec:
      containers:
      - name: transbridge
        image: transbridge:latest
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config-volume
          mountPath: /app/config.yml
          subPath: config.yml
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: transbridge-secrets
              key: openai-api-key
        - name: AUTH_TOKEN
          valueFrom:
            secretKeyRef:
              name: transbridge-secrets
              key: auth-token
      volumes:
      - name: config-volume
        configMap:
          name: transbridge-config
```

3. 创建服务文件 `transbridge-service.yaml`：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: transbridge
spec:
  selector:
    app: transbridge
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

4. 创建密钥：

```bash
kubectl create secret generic transbridge-secrets \
  --from-literal=openai-api-key=your-api-key \
  --from-literal=auth-token=your-auth-token
```

5. 应用配置：

```bash
kubectl apply -f transbridge-config.yaml
kubectl apply -f transbridge-deployment.yaml
kubectl apply -f transbridge-service.yaml
```

## 反向代理配置

### Nginx

```nginx
server {
    listen 80;
    server_name translate.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

使用 HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name translate.yourdomain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 性能优化

1. 使用 Redis 缓存提高性能：

```yaml
cache:
  enabled: true
  types: ["memory", "redis"]
  memory:
    ttl:
      value: "1h"
    max_size: 10000
  redis:
    host: "redis-server"
    port: 6379
    password: "your-password"
    db: 0
    ttl:
      value: "7d"
```

2. 适当增加 API 提供商的并发限制（如果支持）

3. 调整日志级别以减少 I/O 操作

## 监控设置

使用 Prometheus 和 Grafana 监控 TransBridge 的运行状态：

1. 启用 Prometheus 指标端点（在配置文件中添加）：

```yaml
metrics:
  enabled: true
  endpoint: "/metrics"
```

2. 配置 Prometheus 抓取指标：

```yaml
scrape_configs:
  - job_name: 'transbridge'
    scrape_interval: 15s
    static_configs:
      - targets: ['transbridge-host:8080']
```

## 安全建议

为确保 TransBridge 的安全部署，请考虑以下建议：

1. **API 密钥管理**
    - 定期轮换 API 密钥
    - 对密钥使用访问控制和权限管理
    - 避免在代码库或公共场所泄露密钥

2. **网络安全**
    - 始终使用 HTTPS 保护传输层
    - 考虑使用 WAF (Web Application Firewall) 防护
    - 限制仅必要的 IP 地址访问 API 服务

3. **日志和审计**
    - 定期查看日志文件寻找异常模式
    - 设置日志轮转和保留策略，避免日志占用过多磁盘空间
    - 考虑将日志发送到集中式日志管理系统

4. **容错和恢复**
    - 设置自动重启服务
    - 实施监控和报警系统
    - 定期备份配置文件

5. **资源限制**
    - 设置服务的 CPU 和内存限制
    - 配置速率限制，防止 API 滥用
    - 考虑设置连接数限制

## 高可用部署

对于需要高可用性的生产环境，推荐以下部署架构：

```
                     ┌───────────────┐
                     │  Load Balancer│
                     └───────┬───────┘
                             │
         ┌───────────────────┴───────────────────┐
         │                                       │
┌────────▼─────────┐                 ┌───────────▼────────┐
│ TransBridge Node 1│                 │ TransBridge Node 2 │
└────────┬─────────┘                 └───────────┬────────┘
         │                                       │
         └───────────────────┬───────────────────┘
                             │
                     ┌───────▼───────┐
                     │ Redis Cluster │
                     └───────────────┘
```

部署步骤：

1. 设置共享的 Redis 缓存集群
2. 部署多个 TransBridge 实例
3. 配置负载均衡器，如 Nginx, HAProxy 或云服务提供商的负载均衡服务
4. 确保所有实例使用相同的配置（除了端口等实例特定配置）

## 故障排除

### 日志分析

查看日志以排查问题：

```bash
# 查看服务日志
journalctl -u transbridge

# 查看应用日志
tail -f /path/to/translation.log
```

### 常见问题

1. **服务无法启动**
    - 检查配置文件语法
    - 确认端口未被占用
    - 检查权限问题

2. **翻译失败**
    - 检查 API 密钥是否有效
    - 确认网络连接到翻译服务提供商
    - 检查请求格式是否正确

3. **缓存不工作**
    - 检查缓存配置
    - 确认 Redis 服务可用（如使用 Redis）
    - 检查内存使用情况

4. **性能问题**
    - 检查 API 提供商的速率限制
    - 考虑增加缓存配置
    - 检查系统资源利用率

## 更新和迁移

### 版本更新

1. 备份当前配置
   ```bash
   cp config.yml config.yml.backup
   ```

2. 停止当前服务
   ```bash
   sudo systemctl stop transbridge
   ```

3. 替换可执行文件
   ```bash
   cp new-transbridge /opt/transbridge/transbridge
   ```

4. 更新配置（如需要）
   ```bash
   cp new-config.yml /opt/transbridge/config.yml
   ```

5. 启动服务
   ```bash
   sudo systemctl start transbridge
   ```

### 数据迁移

如需将服务迁移到新服务器：

1. 在新服务器上安装 TransBridge
2. 复制配置文件
3. 如果使用 Redis 缓存，可以考虑迁移 Redis 数据（如有必要）
4. 更新 DNS 记录或负载均衡器配置
5. 验证新服务正常工作后，停止旧服务

## 专业支持

如果您在部署过程中遇到问题，可以：

1. 查阅 [项目问题跟踪器](https://github.com/your-username/transbridge/issues)
2. 加入 [社区讨论](https://github.com/your-username/transbridge/discussions)
3. 贡献代码或文档改进

## 进阶使用场景

### 与现有系统集成

TransBridge 可以轻松集成到现有系统中，例如：

1. 作为微服务架构的一部分
2. 为内容管理系统提供翻译能力
3. 为聊天机器人或客服系统提供多语言支持

### 定制开发

TransBridge 设计为易于扩展，如需添加新功能：

1. 添加新的翻译提供商
2. 实现自定义的缓存策略
3. 添加更多的 API 端点

请参考 [CONTRIBUTING.md](../CONTRIBUTING.md) 了解如何贡献代码。

[vercel-deploy]: https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fxiaozhang959%2Ftransbridge&project-name=transbridge&repository-name=transbridge&env=SERVER_PORT,PROVIDER_1_TYPE,PROVIDER_1_API_URL,PROVIDER_1_API_KEY,PROVIDER_1_TIMEOUT,PROVIDER_1_IS_DEFAULT,PROVIDER_1_MODEL_1_NAME,PROVIDER_1_MODEL_1_WEIGHT,PROVIDER_1_MODEL_1_TOP_P,PROVIDER_1_MODEL_1_MAX_TOKENS,PROVIDER_1_MODEL_1_TEMPERATURE,PROVIDER_1_MODEL_2_NAME,PROVIDER_1_MODEL_2_WEIGHT,PROVIDER_1_MODEL_2_TOP_P,PROVIDER_1_MODEL_2_MAX_TOKENS,PROVIDER_1_MODEL_2_TEMPERATURE,CACHE_ENABLED,CACHE_TYPES,CACHE_MEMORY_TTL,CACHE_MEMORY_MAX_SIZE,REDIS_HOST,REDIS_CACHE_PORT,REDIS_PASSWORD,REDIS_TLS,REDIS_DB,REDIS_TTL,PROMPT_TEMPLATE,TRANSAPI_TOKENS,OPENAI_COMPATIBLE_ENABLED,OPENAI_COMPATIBLE_PATH,OPENAI_COMPATIBLE_AUTH_TOKENS,TELEGRAM_ENABLED,TELEGRAM_BOT_TOKEN,TELEGRAM_BOT_USERNAME,TELEGRAM_WEBHOOK_SECRET,TELEGRAM_ALLOWED_CHAT_IDS,TELEGRAM_ALLOWED_USER_IDS,TELEGRAM_DELETE_AFTER_SECONDS,TELEGRAM_POLL_TIMEOUT_SECONDS,TELEGRAM_API_BASE_URL,LOG_ENABLED,LOG_FILE_PATH,LOG_MAX_SIZE,LOG_MAX_AGE,LOG_MAX_BACKUPS,LOG_QUEUE_SIZE&envDescription=%E5%A1%AB%E5%86%99%20PROVIDER_1_API_KEY%20%E5%92%8C%20TRANSAPI_TOKENS%20%E5%8D%B3%E5%8F%AF%E9%83%A8%E7%BD%B2%E7%BF%BB%E8%AF%91%20API%EF%BC%9BTelegram%2FRedis%20%E4%B8%BA%E5%8F%AF%E9%80%89%E8%83%BD%E5%8A%9B%EF%BC%8C%E9%9D%9E%E6%95%8F%E6%84%9F%E9%A1%B9%E5%B7%B2%E9%A2%84%E5%A1%AB%E9%BB%98%E8%AE%A4%E5%80%BC%E3%80%82&envLink=https%3A%2F%2Fgithub.com%2Fxiaozhang959%2Ftransbridge%2Fblob%2Fmain%2FREADME.md%23vercel-%E9%83%A8%E7%BD%B2%E8%AF%B4%E6%98%8E&envDefaults=%7B%22SERVER_PORT%22%3A%228080%22%2C%22PROVIDER_1_TYPE%22%3A%22openai%22%2C%22PROVIDER_1_API_URL%22%3A%22https%3A%2F%2Fapi.openai.com%2Fv1%2Fchat%2Fcompletions%22%2C%22PROVIDER_1_TIMEOUT%22%3A%2230%22%2C%22PROVIDER_1_IS_DEFAULT%22%3A%22true%22%2C%22PROVIDER_1_MODEL_1_NAME%22%3A%22gpt-4o-mini%22%2C%22PROVIDER_1_MODEL_1_WEIGHT%22%3A%2210%22%2C%22PROVIDER_1_MODEL_1_TOP_P%22%3A%221%22%2C%22PROVIDER_1_MODEL_1_MAX_TOKENS%22%3A%224000%22%2C%22PROVIDER_1_MODEL_1_TEMPERATURE%22%3A%220.2%22%2C%22PROVIDER_1_MODEL_2_NAME%22%3A%22gpt-4.1-mini%22%2C%22PROVIDER_1_MODEL_2_WEIGHT%22%3A%225%22%2C%22PROVIDER_1_MODEL_2_TOP_P%22%3A%221%22%2C%22PROVIDER_1_MODEL_2_MAX_TOKENS%22%3A%224000%22%2C%22PROVIDER_1_MODEL_2_TEMPERATURE%22%3A%220.2%22%2C%22CACHE_ENABLED%22%3A%22true%22%2C%22CACHE_TYPES%22%3A%22%5B%5C%22memory%5C%22%5D%22%2C%22CACHE_MEMORY_TTL%22%3A%221h%22%2C%22CACHE_MEMORY_MAX_SIZE%22%3A%2210000%22%2C%22REDIS_HOST%22%3A%22127.0.0.1%22%2C%22REDIS_CACHE_PORT%22%3A%226379%22%2C%22REDIS_TLS%22%3A%22false%22%2C%22REDIS_DB%22%3A%220%22%2C%22REDIS_TTL%22%3A%2224h%22%2C%22PROMPT_TEMPLATE%22%3A%22Translate%20the%20following%20%7B%7Binput%7D%7D%20from%20%7B%7Bsource_lang%7D%7D%20to%20%7B%7Btarget_lang%7D%7D.%20Return%20only%20the%20final%20translation%20result.%22%2C%22OPENAI_COMPATIBLE_ENABLED%22%3A%22false%22%2C%22OPENAI_COMPATIBLE_PATH%22%3A%22%2Fv1%22%2C%22OPENAI_COMPATIBLE_AUTH_TOKENS%22%3A%22%5B%5D%22%2C%22TELEGRAM_ENABLED%22%3A%22false%22%2C%22TELEGRAM_ALLOWED_CHAT_IDS%22%3A%22%5B%5D%22%2C%22TELEGRAM_ALLOWED_USER_IDS%22%3A%22%5B%5D%22%2C%22TELEGRAM_DELETE_AFTER_SECONDS%22%3A%2260%22%2C%22TELEGRAM_POLL_TIMEOUT_SECONDS%22%3A%2230%22%2C%22TELEGRAM_API_BASE_URL%22%3A%22https%3A%2F%2Fapi.telegram.org%22%2C%22LOG_ENABLED%22%3A%22false%22%2C%22LOG_FILE_PATH%22%3A%22logs%2Ftranslation.log%22%2C%22LOG_MAX_SIZE%22%3A%2210%22%2C%22LOG_MAX_AGE%22%3A%2210%22%2C%22LOG_MAX_BACKUPS%22%3A%225%22%2C%22LOG_QUEUE_SIZE%22%3A%2210000%22%7D
