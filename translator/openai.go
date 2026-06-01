package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"transbridge/internal/utils"

	"github.com/sashabaranov/go-openai"
)

// TranslationMetrics 翻译指标
type TranslationMetrics struct {
	InputTokens  int     `json:"input_tokens"`  // 输入token数
	OutputTokens int     `json:"output_tokens"` // 输出token数
	TotalTokens  int     `json:"total_tokens"`  // 总token数
	ModelLatency float64 `json:"model_latency"` // 模型处理延迟（毫秒）
}

// OpenAITranslator 实现 OpenAI 的翻译器
type OpenAITranslator struct {
	Provider    string
	ApiURL      string
	ApiKey      string
	Model       string
	Timeout     int
	MaxTokens   int
	Top_P       float32
	Temperature float32
	Client      *http.Client
	LastMetrics TranslationMetrics
	RetryTimes  int
}

// 确保 OpenAITranslator 实现了 Translator 接口
var _ Translator = (*OpenAITranslator)(nil)

// NewOpenAITranslator 创建新的OpenAI翻译器实例
func NewOpenAITranslator(provider, apiURL, apiKey, model string, timeout, maxTokens int, temperature, topP float32) *OpenAITranslator {
	// 确保默认值合理
	if timeout <= 0 {
		timeout = 30 // 默认30秒超时
	}
	if temperature <= 0 {
		temperature = 0.3 // 默认温度值
	}
	if topP <= 0 {
		topP = 1
	}
	if maxTokens <= 0 {
		maxTokens = 2000 // 默认最大token数
	}

	return &OpenAITranslator{
		Provider:    provider,
		ApiURL:      apiURL,
		ApiKey:      apiKey,
		Model:       model,
		Timeout:     timeout,
		MaxTokens:   maxTokens,
		Top_P:       topP,
		Temperature: temperature,
		Client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		RetryTimes: 2,
	}
}

// Translate 实现翻译功能
func (t *OpenAITranslator) Translate(promptTemplate, text, sourceLang, targetLang string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(t.Timeout)*time.Second)
	defer cancel()

	return t.TranslateWithContext(ctx, promptTemplate, text, sourceLang, targetLang)
}

// TranslateWithContext 支持上下文的翻译方法
func (t *OpenAITranslator) TranslateWithContext(ctx context.Context, promptTemplate, text, sourceLang, targetLang string) (string, error) {
	if strings.TrimSpace(t.ApiURL) == "" {
		return "", fmt.Errorf("provider api url is empty")
	}
	if strings.TrimSpace(t.ApiKey) == "" {
		return "", fmt.Errorf("provider api key is empty")
	}
	if strings.TrimSpace(t.Model) == "" {
		return "", fmt.Errorf("provider model is empty")
	}

	slang, _ := utils.GetLanguageName(sourceLang)
	tlang, _ := utils.GetLanguageName(targetLang)

	prompt, err := utils.ApplyPromptTemplate(promptTemplate, text, slang, tlang)
	if err != nil {
		return "", fmt.Errorf("failed to apply prompt template: %w", err)
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// 构造请求
	reqBody := openai.ChatCompletionRequest{
		Model:       t.Model,
		Messages:    messages,
		TopP:        t.Top_P,
		Temperature: t.Temperature,
		MaxTokens:   t.MaxTokens,
	}

	reqData, errVar := marshalTranslationRequest(reqBody)
	//	log.Println("reqBody: ", reqBody)
	if errVar != nil {
		return "", fmt.Errorf("failed to marshal request: %w", errVar)
	}

	// 发送请求
	var resp *http.Response
	var lastUpstreamError string
	for attempt := 0; attempt <= t.RetryTimes; attempt++ {
		req, errVar := http.NewRequestWithContext(ctx, "POST", t.ApiURL, bytes.NewReader(reqData))
		if errVar != nil {
			return "", fmt.Errorf("failed to create request: %w", errVar)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.ApiKey))

		resp, err = t.Client.Do(req)
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}

		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastUpstreamError = fmt.Sprintf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return "", fmt.Errorf("%s", lastUpstreamError)
			}
		}

		// 指数退避
		backoff := time.Duration(200*(1<<attempt)) * time.Millisecond
		time.Sleep(backoff)
	}
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp == nil {
		if lastUpstreamError != "" {
			return "", fmt.Errorf("%s", lastUpstreamError)
		}
		return "", fmt.Errorf("request failed: empty upstream response")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upstream status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应。先读取 body，便于在上游返回非标准结构时输出安全诊断摘要。
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result openai.ChatCompletionResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		log.Printf(
			"openai compatible response decode failed: api_url=%s model=%s status=%d body=%s",
			safeAPIURL(t.ApiURL),
			t.Model,
			resp.StatusCode,
			summarizeUpstreamBody(responseBody, 800),
		)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// 检查响应是否包含翻译结果
	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		log.Printf(
			"openai compatible response missing translation: api_url=%s model=%s status=%d choices=%d body=%s",
			safeAPIURL(t.ApiURL),
			t.Model,
			resp.StatusCode,
			len(result.Choices),
			summarizeUpstreamBody(responseBody, 800),
		)
		return "", fmt.Errorf("no translation result in response")
	}

	return result.Choices[0].Message.Content, nil
}

// GetProvider 获取提供商名称
func (t *OpenAITranslator) GetProvider() string {
	return t.Provider
}

// GetAPIURL 获取 API URL
func (t *OpenAITranslator) GetAPIURL() string {
	return t.ApiURL
}

// GetModel 获取模型名称
func (t *OpenAITranslator) GetModel() string {
	return t.Model
}

// GetMetrics 获取最近一次请求的指标
func (t *OpenAITranslator) GetMetrics() TranslationMetrics {
	return t.LastMetrics
}

// Close 实现清理接口
func (t *OpenAITranslator) Close() error {
	// OpenAI 客户端当前不需要特别的清理操作
	return nil
}

// ValidateConfig 验证配置是否有效
func (t *OpenAITranslator) ValidateConfig() error {
	if t.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if t.Model == "" {
		return fmt.Errorf("model is required")
	}
	if t.Client == nil {
		return fmt.Errorf("client is not initialized")
	}
	return nil
}

// String 实现 Stringer 接口
func (t *OpenAITranslator) String() string {
	return fmt.Sprintf("%s/%s", t.Provider, t.Model)
}

func marshalTranslationRequest(req openai.ChatCompletionRequest) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	// 某些 OpenAI 兼容服务不按标准处理缺省值；这里显式声明非流式。
	payload["stream"] = false

	return json.Marshal(payload)
}

func safeAPIURL(apiURL string) string {
	if idx := strings.Index(apiURL, "?"); idx >= 0 {
		return apiURL[:idx]
	}
	return apiURL
}

func summarizeUpstreamBody(body []byte, limit int) string {
	summary := strings.TrimSpace(string(body))
	if summary == "" {
		return "<empty>"
	}

	summary = redactSensitiveFields(summary)
	summary = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return ' '
		}
		return r
	}, summary)

	if limit > 0 && len(summary) > limit {
		return summary[:limit] + "...(truncated)"
	}
	return summary
}

var sensitiveJSONFieldPattern = regexp.MustCompile(`(?i)("(?:api[_-]?key|access[_-]?token|token|authorization)"\s*:\s*")[^"]+(")`)

func redactSensitiveFields(text string) string {
	return sensitiveJSONFieldPattern.ReplaceAllString(text, `${1}<redacted>${2}`)
}

// OpenAIChatCompletion 提供 OpenAI 聊天完成功能
type OpenAIChatCompletion struct {
	*OpenAITranslator
}

// NewOpenAIChatCompletion 创建新的 OpenAI 聊天完成实例
func NewOpenAIChatCompletion(translator *OpenAITranslator) *OpenAIChatCompletion {
	return &OpenAIChatCompletion{
		OpenAITranslator: translator,
	}
}

// CreateChatCompletion 提供原生的ChatCompletion接口
func (t *OpenAIChatCompletion) CreateChatCompletion(ctx context.Context, oaiRequest openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	reqData, err := json.Marshal(oaiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		t.ApiURL,
		bytes.NewBuffer(reqData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.ApiKey))

	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
