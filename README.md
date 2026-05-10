# TransBridge 🌉

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

TransBridge 是一个基于 Go 的翻译 API 代理服务。它通过调用大模型
API 提供统一的翻译能力，并兼容部分 DeepL / OpenAI 风格接口，
适合本地部署、二次开发和自定义翻译网关场景。

---

## ✨ 主要特点

- **多提供商支持**：支持配置 OpenAI 兼容接口等翻译提供商
- **多模型加权**：支持同一提供商下的多模型权重分发
- **多级缓存**：支持内存缓存，可按需扩展 Redis 缓存
- **接口兼容**：提供 `/translate`、`/immersivel` 和 OpenAI 兼容接口
- **日志完整**：支持异步日志与日志轮转
- **跨平台**：支持 Linux、macOS、Windows

---

## 🚀 快速开始

### 1. 获取项目

```bash
git clone https://github.com/fruitbars/transbridge.git
cd transbridge
```

### 2. 准备运行文件

推荐把以下文件放在同一个目录中：

```text
transbridge/
├── .env
├── config.yml
└── transbridge-linux-amd64   # 或其他平台二进制
```

### 3. 配置方式说明

当前版本的配置加载顺序如下：

1. 读取 `-config` 指定的 `config.yml`
2. 自动尝试加载 `config.yml` 同目录下的 `.env`
3. 将 `config.yml` 中的 `${VAR}` 占位符替换为环境变量值

这意味着：

- **日常主要维护 `.env`**
- `config.yml` 负责结构模板
- Shell 中显式传入的环境变量优先级 **高于** `.env`

---

## ⚙️ 配置示例

### `.env` 示例

> 建议把真实密钥只放在 `.env` 中，不要直接写进 `config.yml`

```env
SERVER_PORT=8080

PROVIDER_1_TYPE=openai
PROVIDER_1_API_URL=https://api.openai.com/v1/chat/completions
PROVIDER_1_API_KEY=sk-your-api-key
PROVIDER_1_TIMEOUT=30
PROVIDER_1_IS_DEFAULT=true

PROVIDER_1_MODEL_1_NAME=gpt-4o-mini
PROVIDER_1_MODEL_1_WEIGHT=10
PROVIDER_1_MODEL_1_MAX_TOKENS=4000
PROVIDER_1_MODEL_1_TEMPERATURE=0.2

PROVIDER_1_MODEL_2_NAME=gpt-4.1-mini
PROVIDER_1_MODEL_2_WEIGHT=5
PROVIDER_1_MODEL_2_MAX_TOKENS=4000
PROVIDER_1_MODEL_2_TEMPERATURE=0.2

CACHE_ENABLED=true
CACHE_TYPES=["memory"]
CACHE_MEMORY_TTL=1h
CACHE_MEMORY_MAX_SIZE=10000

REDIS_HOST=127.0.0.1
REDIS_CACHE_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_TTL=24h

PROMPT_TEMPLATE=Translate the following {{input}} from {{source_lang}} to {{target_lang}}. Return only the final translation result.

TRANSAPI_TOKENS=["tr-demo-token"]

OPENAI_COMPATIBLE_ENABLED=false
OPENAI_COMPATIBLE_PATH=/v1
OPENAI_COMPATIBLE_AUTH_TOKENS=[]

LOG_ENABLED=true
LOG_FILE_PATH=logs/translation.log
LOG_MAX_SIZE=10
LOG_MAX_AGE=10
LOG_MAX_BACKUPS=5
LOG_QUEUE_SIZE=10000
```

### `config.yml` 示例

```yaml
server:
  port: ${SERVER_PORT}

providers:
  - provider: "${PROVIDER_1_TYPE}"
    api_url: "${PROVIDER_1_API_URL}"
    api_key: "${PROVIDER_1_API_KEY}"
    timeout: ${PROVIDER_1_TIMEOUT}
    is_default: ${PROVIDER_1_IS_DEFAULT}
    models:
      - name: "${PROVIDER_1_MODEL_1_NAME}"
        weight: ${PROVIDER_1_MODEL_1_WEIGHT}
        max_tokens: ${PROVIDER_1_MODEL_1_MAX_TOKENS}
        temperature: ${PROVIDER_1_MODEL_1_TEMPERATURE}
      - name: "${PROVIDER_1_MODEL_2_NAME}"
        weight: ${PROVIDER_1_MODEL_2_WEIGHT}
        max_tokens: ${PROVIDER_1_MODEL_2_MAX_TOKENS}
        temperature: ${PROVIDER_1_MODEL_2_TEMPERATURE}

cache:
  enabled: ${CACHE_ENABLED}
  types: ${CACHE_TYPES}
  memory:
    ttl:
      value: "${CACHE_MEMORY_TTL}"
    max_size: ${CACHE_MEMORY_MAX_SIZE}
  redis:
    host: "${REDIS_HOST}"
    port: ${REDIS_CACHE_PORT}
    password: "${REDIS_PASSWORD}"
    db: ${REDIS_DB}
    ttl:
      value: "${REDIS_TTL}"

prompt:
  template: >-
    ${PROMPT_TEMPLATE}

transapi:
  tokens: ${TRANSAPI_TOKENS}

openai:
  compatible_api:
    enabled: ${OPENAI_COMPATIBLE_ENABLED}
    path: "${OPENAI_COMPATIBLE_PATH}"
    auth_tokens: ${OPENAI_COMPATIBLE_AUTH_TOKENS}

log:
  enabled: ${LOG_ENABLED}
  file_path: "${LOG_FILE_PATH}"
  max_size: ${LOG_MAX_SIZE}
  max_age: ${LOG_MAX_AGE}
  max_backups: ${LOG_MAX_BACKUPS}
  queue_size: ${LOG_QUEUE_SIZE}
```

### 配置注意事项

- 数字和布尔值在 `.env` 中不要额外加引号
- 数组字段需写成 YAML/JSON inline 风格，例如：
  - `CACHE_TYPES=["memory"]`
  - `TRANSAPI_TOKENS=["tr-demo-token"]`
- 如果你要启用 Redis，请把 `CACHE_TYPES` 改成：

```env
CACHE_TYPES=["memory","redis"]
```

并补齐 Redis 相关变量

---

## 🛠️ 构建

### 方式一：Linux / macOS 下使用脚本

```bash
chmod +x build.sh
./build.sh --linux
```

构建产物默认输出到 `dist/`：

- `dist/transbridge-linux-amd64`
- `dist/transbridge-linux-arm64`

### 方式二：Windows 下交叉编译 Ubuntu 可执行文件

在 PowerShell 中执行：

```powershell
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o dist/transbridge-linux-amd64 .
```

如果目标机器是 ARM64：

```powershell
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="arm64"
go build -o dist/transbridge-linux-arm64 .
```

---

## ▶️ 运行

### Linux / Ubuntu

```bash
chmod +x ./transbridge-linux-amd64
./transbridge-linux-amd64 -config ./config.yml
```

如果你想临时覆盖 `.env` 中的值，可以直接这样运行：

```bash
SERVER_PORT=9090 ./transbridge-linux-amd64 -config ./config.yml
```

---

## 📡 API 示例

### 1. `/translate`

```bash
curl -X POST "http://127.0.0.1:8080/translate?token=tr-demo-token" \
  -H "Content-Type: application/json" \
  -d '{
    "text": "你好，世界",
    "source_lang": "zh",
    "target_lang": "en"
  }'
```

### 2. `/immersivel`

```bash
curl -X POST "http://127.0.0.1:8080/immersivel" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer tr-demo-token" \
  -d '{
    "source_lang": "zh",
    "target_lang": "en",
    "text_list": ["需要翻译的内容"]
  }'
```

### 3. OpenAI 兼容接口

当 `OPENAI_COMPATIBLE_ENABLED=true` 时，可使用：

- `POST /v1/chat/completions`
- `GET /v1/models`

---

## 📚 文档

- [配置详解](docs/CONFIGURATION.md)
- [API 文档](docs/API.md)
- [部署指南](docs/DEPLOYMENT.md)

---

## 🔒 安全建议

- 不要把真实 `API Key`、`Token` 直接写进 README 或提交到公开仓库
- 建议将 `.env` 加入 `.gitignore`
- 对外暴露服务时，请至少启用 `transapi.tokens` 或 OpenAI 兼容认证

---

## 🤝 贡献

欢迎提交 Issue / PR 来完善功能、文档和部署体验。

---

## 📜 许可证

本项目采用 MIT License，详见 [LICENSE](LICENSE)

---

## 🙏 致谢

- [go-openai](https://github.com/sashabaranov/go-openai)
- [lumberjack](https://github.com/natefinch/lumberjack)

---

## ⚠️ 免责声明

本项目仅供学习与研究使用，请遵守相关 API 服务提供商的使用条款。
