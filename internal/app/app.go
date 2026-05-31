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

	mux.HandleFunc("/",
		middleware.Chain(
			func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(homePageHTML))
			},
			middleware.Recovery,
			middleware.Logger,
			middleware.CORS,
		),
	)

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

const homePageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>TransBridge · AI 翻译接口</title>
  <style>
    :root {
      --ink: #15201b;
      --muted: #66736b;
      --line: rgba(21, 32, 27, .14);
      --paper: #fbf8ef;
      --card: rgba(255, 255, 255, .74);
      --green: #0e7a4f;
      --lime: #b7f25a;
      --blue: #2147ff;
      --shadow: 0 24px 80px rgba(29, 42, 35, .16);
    }

    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--ink);
      font-family: ui-serif, Georgia, Cambria, "Times New Roman", serif;
      background:
        radial-gradient(circle at 8% 10%, rgba(183, 242, 90, .52), transparent 30%),
        radial-gradient(circle at 92% 18%, rgba(33, 71, 255, .16), transparent 34%),
        linear-gradient(135deg, #fbf8ef 0%, #eef4df 48%, #f7f1df 100%);
    }

    .grain {
      min-height: 100vh;
      background-image: linear-gradient(rgba(21, 32, 27, .035) 1px, transparent 1px),
        linear-gradient(90deg, rgba(21, 32, 27, .035) 1px, transparent 1px);
      background-size: 32px 32px;
      padding: 36px clamp(18px, 4vw, 64px);
    }

    header {
      display: flex;
      justify-content: space-between;
      gap: 24px;
      align-items: center;
      max-width: 1180px;
      margin: 0 auto 54px;
    }

    .brand {
      display: flex;
      gap: 12px;
      align-items: center;
      font-weight: 900;
      letter-spacing: -.03em;
      font-size: 22px;
    }

    .mark {
      width: 42px;
      height: 42px;
      display: grid;
      place-items: center;
      border-radius: 16px;
      background: var(--ink);
      color: var(--lime);
      box-shadow: 8px 8px 0 rgba(14, 122, 79, .24);
    }

    nav {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
      font-family: ui-sans-serif, system-ui, sans-serif;
    }

    nav a {
      color: var(--ink);
      text-decoration: none;
      border: 1px solid var(--line);
      padding: 10px 14px;
      border-radius: 999px;
      background: rgba(255, 255, 255, .5);
      backdrop-filter: blur(10px);
    }

    .hero {
      max-width: 1180px;
      margin: 0 auto;
      display: grid;
      grid-template-columns: minmax(0, .92fr) minmax(320px, 1.08fr);
      gap: clamp(26px, 5vw, 72px);
      align-items: center;
    }

    .kicker {
      display: inline-flex;
      gap: 8px;
      align-items: center;
      padding: 9px 12px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: rgba(255,255,255,.55);
      font-family: ui-sans-serif, system-ui, sans-serif;
      font-size: 13px;
      color: var(--muted);
    }

    h1 {
      margin: 22px 0 18px;
      max-width: 740px;
      font-size: clamp(52px, 9vw, 112px);
      line-height: .86;
      letter-spacing: -.075em;
    }

    .lead {
      max-width: 560px;
      color: var(--muted);
      font: 18px/1.75 ui-sans-serif, system-ui, sans-serif;
    }

    .stats {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 12px;
      margin-top: 34px;
      max-width: 560px;
    }

    .stat {
      border: 1px solid var(--line);
      background: rgba(255,255,255,.48);
      border-radius: 22px;
      padding: 16px;
      font-family: ui-sans-serif, system-ui, sans-serif;
    }
    .stat strong { display: block; font-size: 22px; color: var(--green); }
    .stat span { color: var(--muted); font-size: 12px; }

    .panel {
      border: 1px solid rgba(21,32,27,.16);
      background: var(--card);
      border-radius: 34px;
      box-shadow: var(--shadow);
      padding: clamp(18px, 3vw, 30px);
      backdrop-filter: blur(18px);
      position: relative;
      overflow: hidden;
    }

    .panel::before {
      content: "";
      position: absolute;
      inset: 0 0 auto;
      height: 9px;
      background: linear-gradient(90deg, var(--green), var(--lime), var(--blue));
    }

    .field, .row {
      margin-top: 16px;
      font-family: ui-sans-serif, system-ui, sans-serif;
    }
    label {
      display: block;
      margin-bottom: 8px;
      font-weight: 700;
      font-size: 13px;
      color: #314037;
    }
    textarea, input, select {
      width: 100%;
      border: 1px solid var(--line);
      background: rgba(255,255,255,.82);
      color: var(--ink);
      border-radius: 18px;
      padding: 14px 16px;
      font: 15px/1.5 ui-sans-serif, system-ui, sans-serif;
      outline: none;
    }
    textarea { min-height: 154px; resize: vertical; }
    textarea:focus, input:focus, select:focus { border-color: rgba(14,122,79,.5); box-shadow: 0 0 0 4px rgba(183,242,90,.24); }
    .row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }

    button {
      margin-top: 18px;
      width: 100%;
      border: 0;
      border-radius: 20px;
      padding: 16px 18px;
      cursor: pointer;
      color: white;
      font-weight: 900;
      letter-spacing: .04em;
      background: linear-gradient(135deg, #0e7a4f, #15201b);
      box-shadow: 0 16px 30px rgba(14, 122, 79, .25);
    }
    button:disabled { opacity: .62; cursor: wait; }

    .output {
      min-height: 110px;
      white-space: pre-wrap;
      border: 1px dashed rgba(21,32,27,.22);
      background: rgba(247, 241, 223, .72);
      border-radius: 22px;
      padding: 16px;
      font: 15px/1.65 ui-sans-serif, system-ui, sans-serif;
      color: #24342b;
    }
    .hint { margin-top: 10px; color: var(--muted); font-size: 12px; font-family: ui-sans-serif, system-ui, sans-serif; }

    .features {
      max-width: 1180px;
      margin: 54px auto 0;
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 14px;
    }
    .feature {
      border: 1px solid var(--line);
      border-radius: 26px;
      padding: 20px;
      background: rgba(255,255,255,.46);
      font-family: ui-sans-serif, system-ui, sans-serif;
    }
    .feature b { display: block; margin-bottom: 8px; }
    .feature p { margin: 0; color: var(--muted); line-height: 1.65; }

    @media (max-width: 860px) {
      header, .hero { display: block; }
      nav { justify-content: flex-start; margin-top: 18px; }
      .panel { margin-top: 34px; }
      .stats, .features { grid-template-columns: 1fr; }
      .row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main class="grain">
    <header>
      <div class="brand"><span class="mark">桥</span><span>TransBridge</span></div>
      <nav>
        <a href="/health">Health</a>
        <a href="/v1/models">Models</a>
        <a href="https://github.com/xiaozhang959/transbridge" target="_blank" rel="noreferrer">GitHub</a>
      </nav>
    </header>

    <section class="hero">
      <div>
        <span class="kicker">🌉 AI Translation Gateway · OpenAI Compatible</span>
        <h1>把模型变成翻译接口。</h1>
        <p class="lead">
          TransBridge 将 OpenAI 兼容模型、缓存、鉴权和多接口协议桥接为一个轻量翻译服务。
          你可以直接在这里试用，也可以接入沉浸式翻译、脚本或 Telegram Bot。
        </p>
        <div class="stats">
          <div class="stat"><strong>/translate</strong><span>DeepL 风格单文本接口</span></div>
          <div class="stat"><strong>/immersivel</strong><span>批量翻译接口</span></div>
          <div class="stat"><strong>/v1</strong><span>OpenAI 兼容接口</span></div>
        </div>
      </div>

      <section class="panel" aria-label="在线翻译体验">
        <div class="field">
          <label for="token">API Token</label>
          <input id="token" placeholder="输入 TRANSAPI_TOKENS 中的任意一个 token" autocomplete="off" />
          <div class="hint">Token 只在浏览器本地用于本次请求，不会保存。</div>
        </div>
        <div class="row">
          <div>
            <label for="source">源语言</label>
            <select id="source">
              <option value="auto">Auto</option>
              <option value="ZH">中文</option>
              <option value="EN">English</option>
              <option value="JA">日本語</option>
              <option value="KO">한국어</option>
              <option value="FR">Français</option>
              <option value="DE">Deutsch</option>
            </select>
          </div>
          <div>
            <label for="target">目标语言</label>
            <select id="target">
              <option value="EN">English</option>
              <option value="ZH">中文</option>
              <option value="JA">日本語</option>
              <option value="KO">한국어</option>
              <option value="FR">Français</option>
              <option value="DE">Deutsch</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label for="input">待翻译文本</label>
          <textarea id="input" placeholder="输入需要翻译的文本...">你好，欢迎使用 TransBridge。</textarea>
        </div>
        <button id="translate">开始翻译</button>
        <div class="field">
          <label>翻译结果</label>
          <div id="output" class="output">结果会显示在这里。</div>
        </div>
      </section>
    </section>

    <section class="features">
      <article class="feature"><b>多模型加权</b><p>支持多个 OpenAI 兼容模型按权重分发，便于成本和质量平衡。</p></article>
      <article class="feature"><b>缓存友好</b><p>内存缓存开箱即用，也可以接入 Redis 保存跨实例状态。</p></article>
      <article class="feature"><b>Serverless Ready</b><p>可部署到 Vercel，Telegram Bot 使用 webhook 模式接收消息。</p></article>
    </section>
  </main>

  <script>
    const $ = (id) => document.getElementById(id);
    $("translate").addEventListener("click", async () => {
      const btn = $("translate");
      const output = $("output");
      const token = $("token").value.trim();
      const text = $("input").value.trim();

      if (!token) {
        output.textContent = "请先输入 API Token。";
        return;
      }
      if (!text) {
        output.textContent = "请输入需要翻译的文本。";
        return;
      }

      btn.disabled = true;
      btn.textContent = "翻译中...";
      output.textContent = "正在请求 /translate ...";

      try {
        const response = await fetch("/translate", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + token
          },
          body: JSON.stringify({
            text,
            source_lang: $("source").value,
            target_lang: $("target").value
          })
        });

        const data = await response.json().catch(() => ({}));
        if (!response.ok) {
          output.textContent = data.data || data.message || "请求失败：" + response.status;
          return;
        }
        output.textContent = data.data || JSON.stringify(data, null, 2);
      } catch (error) {
        output.textContent = "请求异常：" + error.message;
      } finally {
        btn.disabled = false;
        btn.textContent = "开始翻译";
      }
    });
  </script>
</body>
</html>`
