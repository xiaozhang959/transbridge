# TransBridge 🌉

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Deploy with Vercel](https://vercel.com/button)][vercel-deploy]

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
REDIS_TLS=false
REDIS_DB=0
REDIS_TTL=24h

PROMPT_TEMPLATE=Translate the following {{input}} from {{source_lang}} to {{target_lang}}. Return only the final translation result.

TRANSAPI_TOKENS=["tr-demo-token"]

OPENAI_COMPATIBLE_ENABLED=false
OPENAI_COMPATIBLE_PATH=/v1
OPENAI_COMPATIBLE_AUTH_TOKENS=[]

TELEGRAM_ENABLED=false
TELEGRAM_BOT_TOKEN=1234567890:your_bot_token
TELEGRAM_BOT_USERNAME=your_bot_username
TELEGRAM_WEBHOOK_SECRET=
TELEGRAM_ALLOWED_CHAT_IDS=[-1001234567890]
TELEGRAM_ALLOWED_USER_IDS=[123456789]
TELEGRAM_DELETE_AFTER_SECONDS=60
TELEGRAM_POLL_TIMEOUT_SECONDS=30
TELEGRAM_API_BASE_URL=https://api.telegram.org

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
    tls: false
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

telegram:
  enabled: ${TELEGRAM_ENABLED}
  bot_token: "${TELEGRAM_BOT_TOKEN}"
  bot_username: "${TELEGRAM_BOT_USERNAME}"
  webhook_secret: "${TELEGRAM_WEBHOOK_SECRET}"
  allowed_chat_ids: ${TELEGRAM_ALLOWED_CHAT_IDS}
  allowed_user_ids: ${TELEGRAM_ALLOWED_USER_IDS}
  delete_after_seconds: ${TELEGRAM_DELETE_AFTER_SECONDS}
  poll_timeout_seconds: ${TELEGRAM_POLL_TIMEOUT_SECONDS}
  api_base_url: "${TELEGRAM_API_BASE_URL}"

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
- Telegram 的 ID 列表同样使用 inline 数组格式，例如：
  - `TELEGRAM_ALLOWED_CHAT_IDS=[-1001234567890]`
  - `TELEGRAM_ALLOWED_USER_IDS=[123456789]`
- 如果你要启用 Redis，请把 `CACHE_TYPES` 改成：

```env
CACHE_TYPES=["memory","redis"]
```

并补齐 Redis 相关变量

### Telegram Bot 说明

当你希望当前 Go 服务直接承担 Telegram Bot 能力时，请配置：

```env
TELEGRAM_ENABLED=true
TELEGRAM_BOT_TOKEN=1234567890:your_bot_token
```

可选限制项：

- `TELEGRAM_ALLOWED_CHAT_IDS`：允许使用机器人的群组列表
- `TELEGRAM_ALLOWED_USER_IDS`：允许使用机器人的用户列表

如果这两个列表都留空（即 `[]`），则默认允许所有会话使用。

支持的 Telegram 行为：

- `/ts <text>`
- `/translate <text>`
- 回复一条消息后发送 `ts` / `translate` / `翻译`
- 直接发送 `ts <text>` / `翻译 <text>`
- `/auto` 开启或关闭当前会话自动翻译
- `/get_user_id`
- `/get_group_id`
- `/start`

### Vercel 部署说明

[![Deploy with Vercel](https://vercel.com/button)][vercel-deploy]

项目已提供 `api/index.go` 和 `vercel.json`，可在 Vercel 上以 Go
Serverless Function 方式运行：

- `/translate`
- `/immersivel`
- `/v1/chat/completions`
- `/v1/models`
- `/telegram/webhook`
- `/health`

在 Vercel 项目的 Environment Variables 中配置 `.env` 示例里的变量。
一键部署按钮会自动列出这些变量，并为非敏感项预填推荐默认值。
`vercel.json` 里也内置了非敏感默认值，敏感项仍需要你手动填写。

最少需要手动填写：

```env
PROVIDER_1_API_KEY=sk-your-api-key
TRANSAPI_TOKENS=["tr-demo-token"]
```

如果使用 Telegram Bot，请额外设置：

```env
TELEGRAM_ENABLED=true
TELEGRAM_BOT_TOKEN=1234567890:your_bot_token
TELEGRAM_BOT_USERNAME=your_bot_username
TELEGRAM_WEBHOOK_SECRET=change-me-to-a-random-secret
```

然后设置 Telegram webhook：

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  -d "url=https://your-vercel-domain.vercel.app/telegram/webhook" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
```

> 说明：Vercel 函数不会常驻运行，因此 Telegram 在 Vercel 上使用
> webhook，而不是本地部署时的 `getUpdates` 长轮询。
> `/auto` 和“上一条消息”功能会优先使用已配置的缓存保存状态；
> 如果只使用内存缓存，冷启动或实例回收后状态可能丢失。

也可以参考 `.env.vercel.example` 手动复制环境变量。

匿名管理员说明：

- Telegram 的匿名管理员消息可能以 `sender_chat` 形式出现
- 如果你希望匿名管理员也能正常触发机器人，**建议把群组 ID 加入**
  `TELEGRAM_ALLOWED_CHAT_IDS`
- 如果没有权限的用户或群组发送消息，机器人会返回对应的 ID 提示，
  便于你加入白名单

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

如果要启用 Telegram Bot：

```bash
TELEGRAM_ENABLED=true ./transbridge-linux-amd64 -config ./config.yml
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

[vercel-deploy]: https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fxiaozhang959%2FaiTranslate&project-name=ai-translate&repository-name=aiTranslate&env=SERVER_PORT,PROVIDER_1_TYPE,PROVIDER_1_API_URL,PROVIDER_1_API_KEY,PROVIDER_1_TIMEOUT,PROVIDER_1_IS_DEFAULT,PROVIDER_1_MODEL_1_NAME,PROVIDER_1_MODEL_1_WEIGHT,PROVIDER_1_MODEL_1_MAX_TOKENS,PROVIDER_1_MODEL_1_TEMPERATURE,PROVIDER_1_MODEL_2_NAME,PROVIDER_1_MODEL_2_WEIGHT,PROVIDER_1_MODEL_2_MAX_TOKENS,PROVIDER_1_MODEL_2_TEMPERATURE,CACHE_ENABLED,CACHE_TYPES,CACHE_MEMORY_TTL,CACHE_MEMORY_MAX_SIZE,REDIS_HOST,REDIS_CACHE_PORT,REDIS_PASSWORD,REDIS_TLS,REDIS_DB,REDIS_TTL,PROMPT_TEMPLATE,TRANSAPI_TOKENS,OPENAI_COMPATIBLE_ENABLED,OPENAI_COMPATIBLE_PATH,OPENAI_COMPATIBLE_AUTH_TOKENS,TELEGRAM_ENABLED,TELEGRAM_BOT_TOKEN,TELEGRAM_BOT_USERNAME,TELEGRAM_WEBHOOK_SECRET,TELEGRAM_ALLOWED_CHAT_IDS,TELEGRAM_ALLOWED_USER_IDS,TELEGRAM_DELETE_AFTER_SECONDS,TELEGRAM_POLL_TIMEOUT_SECONDS,TELEGRAM_API_BASE_URL,LOG_ENABLED,LOG_FILE_PATH,LOG_MAX_SIZE,LOG_MAX_AGE,LOG_MAX_BACKUPS,LOG_QUEUE_SIZE&envDescription=%E5%A1%AB%E5%86%99%20PROVIDER_1_API_KEY%20%E5%92%8C%20TRANSAPI_TOKENS%20%E5%8D%B3%E5%8F%AF%E9%83%A8%E7%BD%B2%E7%BF%BB%E8%AF%91%20API%EF%BC%9BTelegram%2FRedis%20%E4%B8%BA%E5%8F%AF%E9%80%89%E8%83%BD%E5%8A%9B%EF%BC%8C%E9%9D%9E%E6%95%8F%E6%84%9F%E9%A1%B9%E5%B7%B2%E9%A2%84%E5%A1%AB%E9%BB%98%E8%AE%A4%E5%80%BC%E3%80%82&envLink=https%3A%2F%2Fgithub.com%2Fxiaozhang959%2FaiTranslate%2Fblob%2Fmain%2FREADME.md%23vercel-%E9%83%A8%E7%BD%B2%E8%AF%B4%E6%98%8E&envDefaults=%7B%22SERVER_PORT%22%3A%228080%22%2C%22PROVIDER_1_TYPE%22%3A%22openai%22%2C%22PROVIDER_1_API_URL%22%3A%22https%3A%2F%2Fapi.openai.com%2Fv1%2Fchat%2Fcompletions%22%2C%22PROVIDER_1_TIMEOUT%22%3A%2230%22%2C%22PROVIDER_1_IS_DEFAULT%22%3A%22true%22%2C%22PROVIDER_1_MODEL_1_NAME%22%3A%22gpt-4o-mini%22%2C%22PROVIDER_1_MODEL_1_WEIGHT%22%3A%2210%22%2C%22PROVIDER_1_MODEL_1_MAX_TOKENS%22%3A%224000%22%2C%22PROVIDER_1_MODEL_1_TEMPERATURE%22%3A%220.2%22%2C%22PROVIDER_1_MODEL_2_NAME%22%3A%22gpt-4.1-mini%22%2C%22PROVIDER_1_MODEL_2_WEIGHT%22%3A%225%22%2C%22PROVIDER_1_MODEL_2_MAX_TOKENS%22%3A%224000%22%2C%22PROVIDER_1_MODEL_2_TEMPERATURE%22%3A%220.2%22%2C%22CACHE_ENABLED%22%3A%22true%22%2C%22CACHE_TYPES%22%3A%22%5B%5C%22memory%5C%22%5D%22%2C%22CACHE_MEMORY_TTL%22%3A%221h%22%2C%22CACHE_MEMORY_MAX_SIZE%22%3A%2210000%22%2C%22REDIS_HOST%22%3A%22127.0.0.1%22%2C%22REDIS_CACHE_PORT%22%3A%226379%22%2C%22REDIS_TLS%22%3A%22false%22%2C%22REDIS_DB%22%3A%220%22%2C%22REDIS_TTL%22%3A%2224h%22%2C%22PROMPT_TEMPLATE%22%3A%22Translate%20the%20following%20%7B%7Binput%7D%7D%20from%20%7B%7Bsource_lang%7D%7D%20to%20%7B%7Btarget_lang%7D%7D.%20Return%20only%20the%20final%20translation%20result.%22%2C%22OPENAI_COMPATIBLE_ENABLED%22%3A%22false%22%2C%22OPENAI_COMPATIBLE_PATH%22%3A%22%2Fv1%22%2C%22OPENAI_COMPATIBLE_AUTH_TOKENS%22%3A%22%5B%5D%22%2C%22TELEGRAM_ENABLED%22%3A%22false%22%2C%22TELEGRAM_ALLOWED_CHAT_IDS%22%3A%22%5B%5D%22%2C%22TELEGRAM_ALLOWED_USER_IDS%22%3A%22%5B%5D%22%2C%22TELEGRAM_DELETE_AFTER_SECONDS%22%3A%2260%22%2C%22TELEGRAM_POLL_TIMEOUT_SECONDS%22%3A%2230%22%2C%22TELEGRAM_API_BASE_URL%22%3A%22https%3A%2F%2Fapi.telegram.org%22%2C%22LOG_ENABLED%22%3A%22false%22%2C%22LOG_FILE_PATH%22%3A%22logs%2Ftranslation.log%22%2C%22LOG_MAX_SIZE%22%3A%2210%22%2C%22LOG_MAX_AGE%22%3A%2210%22%2C%22LOG_MAX_BACKUPS%22%3A%225%22%2C%22LOG_QUEUE_SIZE%22%3A%2210000%22%7D
