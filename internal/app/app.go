package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"transbridge/api/deeplx/translate_handler"
	"transbridge/api/openai"
	"transbridge/cache"
	"transbridge/config"
	"transbridge/internal/middleware"
	"transbridge/logger"
	"transbridge/service"
	"transbridge/telegrambot"
	"transbridge/translator"
)

// Options 控制应用在不同运行环境下的初始化方式。
type Options struct {
	Serverless bool
}

// App 持有 HTTP API、翻译服务和可选的 Telegram Bot。
type App struct {
	Config             *config.Config
	Handler            http.Handler
	TelegramBot        *telegrambot.Bot
	translationLogger  *logger.TranslationLogger
	cache              cache.Cache
	translationService *service.TranslationService
	modelManager       *translator.ModelManager
}

// New 从配置文件初始化应用。该函数不启动监听，便于本地服务和 Vercel 复用。
func New(configFile string, opts Options) (*App, error) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	var cacheImpl cache.Cache
	if cfg.Cache.Enabled {
		if cacheImpl, err = initCache(cfg); err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %w", err)
		}
	}

	var translLogger *logger.TranslationLogger
	if cfg.Log.Enabled && !opts.Serverless {
		loggerOpts := logger.LoggerOptions{
			Enabled:     cfg.Log.Enabled,
			LogFilePath: cfg.Log.FilePath,
			MaxSize:     cfg.Log.MaxSize,
			MaxAge:      cfg.Log.MaxAge,
			MaxBackups:  cfg.Log.MaxBackups,
			QueueSize:   cfg.Log.QueueSize,
		}

		translLogger, err = logger.NewTranslationLogger(loggerOpts)
		if err != nil {
			log.Printf("Warning: Failed to initialize translation logger: %v", err)
		} else {
			log.Printf("Translation logger initialized: %s", cfg.Log.FilePath)
		}
	}

	modelManager, err := translator.NewModelManager(cfg.Providers)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize model manager: %w", err)
	}

	translationService := service.NewTranslationService(modelManager, cacheImpl, translLogger)

	bot, err := telegrambot.NewWithState(cfg.Telegram, translationService, cfg.Prompt.Template, cacheImpl)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	app := &App{
		Config:             cfg,
		TelegramBot:        bot,
		translationLogger:  translLogger,
		cache:              cacheImpl,
		translationService: translationService,
		modelManager:       modelManager,
	}
	app.Handler = app.setupMux()

	return app, nil
}

// HTTPServer 返回本地常驻服务使用的 http.Server。
func (a *App) HTTPServer() *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port),
		Handler:      a.Handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Close 释放本地常驻服务使用的资源。
func (a *App) Close(ctx context.Context) error {
	if a.translationLogger != nil {
		if err := a.translationLogger.Close(); err != nil {
			return err
		}
	}
	if a.cache != nil {
		return a.cache.Close(ctx)
	}
	return nil
}

func (a *App) setupMux() http.Handler {
	mux := http.NewServeMux()

	translationHandler := translate_handler.NewHandler(a.translationService, translate_handler.HandlerConfig{
		AuthTokens:     a.Config.TransAPI.Tokens,
		PromptTemplate: a.Config.Prompt.Template,
	})

	mux.HandleFunc("/translate",
		middleware.Chain(
			translationHandler.HandleTranslation,
			middleware.Recovery,
			middleware.Logger,
			middleware.CORS,
		),
	)

	mux.HandleFunc("/immersivel",
		middleware.Chain(
			translationHandler.HandleImmersiveLTranslation,
			middleware.Recovery,
			middleware.Logger,
			middleware.CORS,
		),
	)

	if a.Config.OpenAI.CompatibleAPI.Enabled {
		openaiHandler := openai.NewOpenAIHandler(a.modelManager, a.Config.OpenAI.CompatibleAPI.AuthTokens)

		mux.HandleFunc("/v1/chat/completions",
			middleware.Chain(
				openaiHandler.HandleChatCompletion,
				middleware.Recovery,
				middleware.Logger,
				middleware.CORS,
			),
		)

		mux.HandleFunc("/v1/models",
			middleware.Chain(
				openaiHandler.HandleListModels,
				middleware.Recovery,
				middleware.Logger,
				middleware.CORS,
			),
		)
	}

	if a.TelegramBot != nil {
		mux.HandleFunc("/telegram/webhook",
			middleware.Chain(
				a.TelegramBot.HandleWebhook,
				middleware.Recovery,
				middleware.Logger,
			),
		)
	}

	mux.HandleFunc("/health",
		middleware.Chain(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			},
			middleware.Logger,
		),
	)

	return mux
}

func initCache(cfg *config.Config) (cache.Cache, error) {
	var caches []cache.Cache

	for _, cacheType := range cfg.Cache.Types {
		switch cacheType {
		case "memory":
			ttl := time.Hour
			isPermanent := false

			if duration, ok := cfg.Cache.Memory.TTL.Duration(); ok {
				if duration < 0 {
					isPermanent = true
				} else {
					ttl = duration
				}
			}

			maxSize := cfg.Cache.Memory.MaxSize
			if maxSize <= 0 {
				maxSize = 10000
			}

			caches = append(caches, cache.NewMemoryCache(cache.MemoryCacheOptions{
				MaxSize:    maxSize,
				DefaultTTL: ttl,
				Permanent:  isPermanent,
			}))

		case "redis":
			ttl := 24 * time.Hour
			isPermanent := false

			if duration, ok := cfg.Cache.Redis.TTL.Duration(); ok {
				if duration < 0 {
					isPermanent = true
				} else {
					ttl = duration
				}
			}

			caches = append(caches, cache.NewRedisCache(cache.RedisCacheOptions{
				Host:       cfg.Cache.Redis.Host,
				Port:       cfg.Cache.Redis.Port,
				Password:   cfg.Cache.Redis.Password,
				DB:         cfg.Cache.Redis.DB,
				TLS:        cfg.Cache.Redis.TLS,
				DefaultTTL: ttl,
				Permanent:  isPermanent,
			}))

		default:
			return nil, fmt.Errorf("unsupported cache type: %s", cacheType)
		}
	}

	if len(caches) == 0 {
		return nil, fmt.Errorf("no cache configured")
	}

	return cache.NewMultiCache(caches), nil
}
