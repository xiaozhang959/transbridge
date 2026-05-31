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

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
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
