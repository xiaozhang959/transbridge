package config

import (
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Providers []ProviderConfig `yaml:"providers"`
	Cache     CacheConfig      `yaml:"cache"`
	Prompt    PromptConfig     `yaml:"prompt"`
	OpenAI    OpenAIConfig     `yaml:"openai"`
	TransAPI  TransAPI         `yaml:"transapi"`
	Telegram  TelegramConfig   `yaml:"telegram"`
	Log       LogConfig        `yaml:"log"`
}

type LogConfig struct {
	Enabled    bool   `yaml:"enabled"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxAge     int    `yaml:"max_age"`
	MaxBackups int    `yaml:"max_backups"`
	QueueSize  int    `yaml:"queue_size"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type ProviderConfig struct {
	Provider  string        `yaml:"provider"`
	APIURL    string        `yaml:"api_url"`
	APIKey    string        `yaml:"api_key"`
	Timeout   int           `yaml:"timeout"`
	IsDefault bool          `yaml:"is_default"`
	Models    []ModelConfig `yaml:"models"`
}

type ModelConfig struct {
	Name        string  `yaml:"name"`
	Weight      int     `yaml:"weight"`
	TopP        float32 `yaml:"top_p"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float32 `yaml:"temperature"`
	Timeout     *int    `yaml:"timeout,omitempty"`
}

type CacheConfig struct {
	Enabled bool         `yaml:"enabled"`
	Types   []string     `yaml:"types"`
	Memory  MemoryConfig `yaml:"memory"`
	Redis   RedisConfig  `yaml:"redis"`
}

type MemoryConfig struct {
	TTL     TTL `yaml:"ttl"`
	MaxSize int `yaml:"max_size"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	TLS      bool   `yaml:"tls"`
	TTL      TTL    `yaml:"ttl"`
}

type PromptConfig struct {
	Template string `yaml:"template"`
}

type OpenAIConfig struct {
	CompatibleAPI struct {
		Enabled    bool     `yaml:"enabled"`
		Path       string   `yaml:"path"`
		AuthTokens []string `yaml:"auth_tokens"`
	} `yaml:"compatible_api"`
}

type StorageConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Type     string `yaml:"type"`
	Path     string `yaml:"path"`
	LogLevel string `yaml:"log_level"`
}

type TransAPI struct {
	Tokens []string `yaml:"tokens"`
}

type TelegramConfig struct {
	Enabled            bool    `yaml:"enabled"`
	BotToken           string  `yaml:"bot_token"`
	BotUsername        string  `yaml:"bot_username"`
	WebhookSecret      string  `yaml:"webhook_secret"`
	AllowedChatIDs     []int64 `yaml:"allowed_chat_ids"`
	AllowedUserIDs     []int64 `yaml:"allowed_user_ids"`
	DeleteAfterSeconds int     `yaml:"delete_after_seconds"`
	PollTimeoutSeconds int     `yaml:"poll_timeout_seconds"`
	APIBaseURL         string  `yaml:"api_base_url"`
}

// LoadConfig loads .env from the config directory, then reads YAML and
// expands ${VAR} placeholders with environment variables.
func LoadConfig(filename string) (*Config, error) {
	envPath := filepath.Join(filepath.Dir(filename), ".env")
	if err := loadDotEnvFile(envPath); err != nil {
		return nil, err
	}
	applyDefaultEnvValues()

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) && os.Getenv("VERCEL") != "" {
			data = []byte(defaultConfigYAML)
		} else {
			return nil, err
		}
	}

	data = []byte(expandEnvPlaceholders(string(data)))

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if value, ok := os.LookupEnv("REDIS_TLS"); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			config.Cache.Redis.TLS = parsed
		}
	}
	clearUnexpandedOptionalString(&config.Telegram.BotUsername)
	clearUnexpandedOptionalString(&config.Telegram.WebhookSecret)

	return &config, nil
}

func clearUnexpandedOptionalString(value *string) {
	if value == nil {
		return
	}
	if envPattern.MatchString(*value) {
		*value = ""
	}
}

const defaultConfigYAML = `
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
        top_p: ${PROVIDER_1_MODEL_1_TOP_P}
        max_tokens: ${PROVIDER_1_MODEL_1_MAX_TOKENS}
        temperature: ${PROVIDER_1_MODEL_1_TEMPERATURE}
      - name: "${PROVIDER_1_MODEL_2_NAME}"
        weight: ${PROVIDER_1_MODEL_2_WEIGHT}
        top_p: ${PROVIDER_1_MODEL_2_TOP_P}
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
`

func applyDefaultEnvValues() {
	defaults := map[string]string{
		"SERVER_PORT":                    "8080",
		"PROVIDER_1_TYPE":                "openai",
		"PROVIDER_1_API_URL":             "https://api.openai.com/v1/chat/completions",
		"PROVIDER_1_API_KEY":             "",
		"PROVIDER_1_TIMEOUT":             "30",
		"PROVIDER_1_IS_DEFAULT":          "true",
		"PROVIDER_1_MODEL_1_NAME":        "gpt-4o-mini",
		"PROVIDER_1_MODEL_1_WEIGHT":      "10",
		"PROVIDER_1_MODEL_1_TOP_P":       "1",
		"PROVIDER_1_MODEL_1_MAX_TOKENS":  "4000",
		"PROVIDER_1_MODEL_1_TEMPERATURE": "0.2",
		"PROVIDER_1_MODEL_2_NAME":        "gpt-4.1-mini",
		"PROVIDER_1_MODEL_2_WEIGHT":      "5",
		"PROVIDER_1_MODEL_2_TOP_P":       "1",
		"PROVIDER_1_MODEL_2_MAX_TOKENS":  "4000",
		"PROVIDER_1_MODEL_2_TEMPERATURE": "0.2",
		"CACHE_ENABLED":                  "true",
		"CACHE_TYPES":                    "[\"memory\"]",
		"CACHE_MEMORY_TTL":               "1h",
		"CACHE_MEMORY_MAX_SIZE":          "10000",
		"REDIS_HOST":                     "127.0.0.1",
		"REDIS_CACHE_PORT":               "6379",
		"REDIS_PASSWORD":                 "",
		"REDIS_TLS":                      "false",
		"REDIS_DB":                       "0",
		"REDIS_TTL":                      "24h",
		"PROMPT_TEMPLATE":                "Translate the following {{input}} from {{source_lang}} to {{target_lang}}. Return only the final translation result.",
		"TRANSAPI_TOKENS":                "[]",
		"OPENAI_COMPATIBLE_ENABLED":      "false",
		"OPENAI_COMPATIBLE_PATH":         "/v1",
		"OPENAI_COMPATIBLE_AUTH_TOKENS":  "[]",
		"TELEGRAM_ENABLED":               "false",
		"TELEGRAM_BOT_TOKEN":             "",
		"TELEGRAM_BOT_USERNAME":          "",
		"TELEGRAM_WEBHOOK_SECRET":        "",
		"TELEGRAM_ALLOWED_CHAT_IDS":      "[]",
		"TELEGRAM_ALLOWED_USER_IDS":      "[]",
		"TELEGRAM_DELETE_AFTER_SECONDS":  "60",
		"TELEGRAM_POLL_TIMEOUT_SECONDS":  "30",
		"TELEGRAM_API_BASE_URL":          "https://api.telegram.org",
		"LOG_ENABLED":                    "false",
		"LOG_FILE_PATH":                  "logs/translation.log",
		"LOG_MAX_SIZE":                   "10",
		"LOG_MAX_AGE":                    "10",
		"LOG_MAX_BACKUPS":                "5",
		"LOG_QUEUE_SIZE":                 "10000",
	}

	for key, value := range defaults {
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
