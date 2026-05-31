package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"transbridge/cache"
	"transbridge/config"
	"transbridge/service"
	"unicode"
)

const (
	defaultTelegramAPIBaseURL = "https://api.telegram.org"
	defaultDeleteAfterSeconds = 60
	defaultPollTimeoutSeconds = 30
	defaultReplyDeleteNotice  = "（此消息将于 %d 秒后自动删除）"
)

type Bot struct {
	cfg                config.TelegramConfig
	translationService *service.TranslationService
	promptTemplate     string
	client             *apiClient
	username           string
	state              cache.Cache

	mu            sync.RWMutex
	lastMessages  map[userChatKey]messageSnapshot
	autoTranslate map[userChatKey]bool
	allowedChats  map[int64]struct{}
	allowedUsers  map[int64]struct{}
}

type userChatKey struct {
	SenderID int64
	ChatID   int64
}

type messageSnapshot struct {
	ChatID    int64
	MessageID int64
	Text      string
}

type senderContext struct {
	ChatID       int64
	UserID       int64
	SenderChatID int64
	KeyID        int64
	Ignore       bool
}

func New(cfg config.TelegramConfig, translationService *service.TranslationService, promptTemplate string) (*Bot, error) {
	return NewWithState(cfg, translationService, promptTemplate, nil)
}

func NewWithState(cfg config.TelegramConfig, translationService *service.TranslationService, promptTemplate string, state cache.Cache) (*Bot, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if strings.TrimSpace(cfg.BotToken) == "" {
		return nil, fmt.Errorf("telegram bot token is required when telegram bot is enabled")
	}

	if cfg.DeleteAfterSeconds <= 0 {
		cfg.DeleteAfterSeconds = defaultDeleteAfterSeconds
	}
	if cfg.PollTimeoutSeconds <= 0 {
		cfg.PollTimeoutSeconds = defaultPollTimeoutSeconds
	}
	if strings.TrimSpace(cfg.APIBaseURL) == "" {
		cfg.APIBaseURL = defaultTelegramAPIBaseURL
	}

	bot := &Bot{
		cfg:                cfg,
		translationService: translationService,
		promptTemplate:     promptTemplate,
		client:             newAPIClient(cfg.APIBaseURL, cfg.BotToken),
		username:           strings.TrimPrefix(strings.TrimSpace(cfg.BotUsername), "@"),
		state:              state,
		lastMessages:       make(map[userChatKey]messageSnapshot),
		autoTranslate:      make(map[userChatKey]bool),
		allowedChats:       make(map[int64]struct{}),
		allowedUsers:       make(map[int64]struct{}),
	}

	for _, id := range cfg.AllowedChatIDs {
		bot.allowedChats[id] = struct{}{}
	}
	for _, id := range cfg.AllowedUserIDs {
		bot.allowedUsers[id] = struct{}{}
	}

	return bot, nil
}

func (b *Bot) Run(ctx context.Context) error {
	if err := b.ensureUsername(ctx); err != nil {
		return err
	}
	log.Printf("Telegram bot started: @%s", b.username)

	var offset int64
	for {
		select {
		case <-ctx.Done():
			log.Println("Telegram bot stopped")
			return nil
		default:
		}

		updates, err := b.client.getUpdates(ctx, offset, b.cfg.PollTimeoutSeconds)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			log.Printf("Telegram polling failed: %v", err)
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			if update.Message == nil {
				continue
			}

			if err := b.handleMessage(ctx, update.Message); err != nil {
				log.Printf("Telegram message handling failed: %v", err)
			}
		}
	}
}

func (b *Bot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if secret := strings.TrimSpace(b.cfg.WebhookSecret); secret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	if err := b.ensureUsername(r.Context()); err != nil {
		log.Printf("get telegram bot profile failed: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	var update telegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid telegram update"))
		return
	}

	if update.Message != nil {
		if err := b.handleMessage(r.Context(), update.Message); err != nil {
			log.Printf("Telegram webhook message handling failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (b *Bot) ensureUsername(ctx context.Context) error {
	b.mu.RLock()
	username := strings.TrimSpace(b.username)
	b.mu.RUnlock()
	if username != "" {
		return nil
	}

	me, err := b.client.getMe(ctx)
	if err != nil {
		return fmt.Errorf("get telegram bot profile failed: %w", err)
	}

	b.mu.Lock()
	if strings.TrimSpace(b.username) == "" {
		b.username = me.UserName
	}
	b.mu.Unlock()

	return nil
}

func (b *Bot) handleMessage(ctx context.Context, message *telegramMessage) error {
	if message == nil {
		return nil
	}

	sender := resolveSender(message)
	if sender.Ignore {
		return nil
	}

	if !b.isAllowed(sender.ChatID, sender.UserID, sender.SenderChatID) {
		return b.replyUnauthorized(ctx, message, sender)
	}

	content := strings.TrimSpace(extractMessageText(message))
	if content == "" {
		return nil
	}

	if command, args, ok := parseSlashCommand(content, b.username); ok {
		return b.handleCommand(ctx, message, command, args)
	}

	if b.handleReplyTrigger(ctx, message, content) {
		return nil
	}

	if b.handleMentionTrigger(ctx, message, content) {
		return nil
	}

	if b.handleTextPrefixTrigger(ctx, message, content) {
		return nil
	}

	b.recordLastMessage(ctx, message)

	if b.isAutoTranslateEnabled(ctx, sender.ChatID, sender.KeyID) {
		return b.translateAndReply(ctx, message, content, replyOptions{
			ReplyToMessageID: message.MessageID,
			AutoDelete:       true,
		})
	}

	return nil
}

func (b *Bot) handleCommand(ctx context.Context, message *telegramMessage, command, args string) error {
	switch command {
	case "start":
		return b.replyText(ctx, message.Chat.ID, message.MessageID, startMessage(), false)
	case "get_user_id":
		sender := resolveSender(message)
		return b.replyText(ctx, message.Chat.ID, message.MessageID, formatSenderIDMessage(sender), false)
	case "get_group_id":
		if message.Chat.ID < 0 {
			return b.replyText(ctx, message.Chat.ID, message.MessageID, fmt.Sprintf("这个群组的ID是: %d", message.Chat.ID), false)
		}
		return b.replyText(ctx, message.Chat.ID, message.MessageID, "这个命令只能在群组中使用。", false)
	case "auto":
		sender := resolveSender(message)
		enabled := b.toggleAutoTranslate(ctx, message.Chat.ID, sender.KeyID)
		status := "开启"
		if !enabled {
			status = "关闭"
		}
		return b.replyText(ctx, message.Chat.ID, message.MessageID, fmt.Sprintf("已%s默认翻译当前会话中的消息", status), false)
	case "ts", "translate":
		return b.handleTranslateCommand(ctx, message, args)
	default:
		return nil
	}
}

func (b *Bot) handleTranslateCommand(ctx context.Context, message *telegramMessage, args string) error {
	text := strings.TrimSpace(args)
	if text != "" {
		return b.translateAndReply(ctx, message, text, replyOptions{
			ReplyToMessageID: message.MessageID,
		})
	}

	if message.ReplyToMessage != nil {
		replyText := strings.TrimSpace(extractMessageText(message.ReplyToMessage))
		if replyText != "" {
			err := b.translateAndReply(ctx, message, replyText, replyOptions{
				ReplyToMessageID: message.ReplyToMessage.MessageID,
			})
			if err == nil {
				_ = b.client.deleteMessage(ctx, message.Chat.ID, message.MessageID)
			}
			return err
		}
	}

	sender := resolveSender(message)
	lastMessage, ok := b.getLastMessage(ctx, message.Chat.ID, sender.KeyID)
	if !ok || strings.TrimSpace(lastMessage.Text) == "" {
		return b.replyText(ctx, message.Chat.ID, message.MessageID, "没有可供翻译的上一条消息。", false)
	}

	err := b.translateAndReply(ctx, message, lastMessage.Text, replyOptions{
		ReplyToMessageID: lastMessage.MessageID,
	})
	if err == nil {
		_ = b.client.deleteMessage(ctx, message.Chat.ID, message.MessageID)
	}
	return err
}

func (b *Bot) handleReplyTrigger(ctx context.Context, message *telegramMessage, content string) bool {
	if message.ReplyToMessage == nil || !isReplyTranslateTrigger(content) {
		return false
	}

	replyText := strings.TrimSpace(extractMessageText(message.ReplyToMessage))
	if replyText == "" {
		return true
	}

	if err := b.translateAndReply(ctx, message, replyText, replyOptions{
		ReplyToMessageID: message.ReplyToMessage.MessageID,
		AutoDelete:       true,
	}); err != nil {
		log.Printf("reply trigger translate failed: %v", err)
	}

	_ = b.client.deleteMessage(ctx, message.Chat.ID, message.MessageID)
	return true
}

func (b *Bot) handleMentionTrigger(ctx context.Context, message *telegramMessage, content string) bool {
	if strings.TrimSpace(b.username) == "" {
		return false
	}

	mention := "@" + b.username
	if !strings.Contains(content, mention) {
		return false
	}

	text := strings.TrimSpace(strings.ReplaceAll(content, mention, ""))
	if text != "" {
		if err := b.translateAndReply(ctx, message, text, replyOptions{
			ReplyToMessageID: message.MessageID,
		}); err != nil {
			log.Printf("mention translate failed: %v", err)
		}
		return true
	}

	sender := resolveSender(message)
	lastMessage, ok := b.getLastMessage(ctx, message.Chat.ID, sender.KeyID)
	if !ok || strings.TrimSpace(lastMessage.Text) == "" {
		return true
	}

	if err := b.translateAndReply(ctx, message, lastMessage.Text, replyOptions{
		ReplyToMessageID: lastMessage.MessageID,
	}); err != nil {
		log.Printf("mention fallback translate failed: %v", err)
	}

	_ = b.client.deleteMessage(ctx, message.Chat.ID, message.MessageID)
	return true
}

func (b *Bot) handleTextPrefixTrigger(ctx context.Context, message *telegramMessage, content string) bool {
	trigger, text := parseTextTranslateTrigger(content)
	if !trigger {
		return false
	}

	if strings.TrimSpace(text) != "" {
		if err := b.translateAndReply(ctx, message, text, replyOptions{
			ReplyToMessageID: message.MessageID,
		}); err != nil {
			log.Printf("text prefix translate failed: %v", err)
		}
		return true
	}

	sender := resolveSender(message)
	lastMessage, ok := b.getLastMessage(ctx, message.Chat.ID, sender.KeyID)
	if !ok || strings.TrimSpace(lastMessage.Text) == "" {
		return true
	}

	if err := b.translateAndReply(ctx, message, lastMessage.Text, replyOptions{
		ReplyToMessageID: lastMessage.MessageID,
		AutoDelete:       true,
	}); err != nil {
		log.Printf("text prefix fallback translate failed: %v", err)
	}

	_ = b.client.deleteMessage(ctx, message.Chat.ID, message.MessageID)
	return true
}

func (b *Bot) translateAndReply(ctx context.Context, message *telegramMessage, sourceText string, options replyOptions) error {
	translation, err := b.translationService.Translate(
		ctx,
		"",
		"",
		b.promptTemplate,
		sourceText,
		"auto",
		detectTargetLanguage(sourceText),
	)
	if err != nil {
		return b.replyText(ctx, message.Chat.ID, message.MessageID, fmt.Sprintf("翻译失败：%v", err), false)
	}

	text := translation
	if options.AutoDelete {
		text = fmt.Sprintf("%s\n\n"+defaultReplyDeleteNotice, translation, b.cfg.DeleteAfterSeconds)
	}

	sentMessageID, err := b.client.sendMessage(ctx, sendMessageRequest{
		ChatID:              message.Chat.ID,
		Text:                text,
		ReplyToMessageID:    options.ReplyToMessageID,
		AllowWithoutReply:   true,
		DisableNotification: false,
	})
	if err != nil {
		return err
	}

	if options.AutoDelete {
		b.scheduleDelete(ctx, message.Chat.ID, sentMessageID)
	}

	return nil
}

func (b *Bot) replyText(ctx context.Context, chatID, replyToMessageID int64, text string, autoDelete bool) error {
	sentMessageID, err := b.client.sendMessage(ctx, sendMessageRequest{
		ChatID:              chatID,
		Text:                text,
		ReplyToMessageID:    replyToMessageID,
		AllowWithoutReply:   true,
		DisableNotification: false,
	})
	if err != nil {
		return err
	}

	if autoDelete {
		b.scheduleDelete(ctx, chatID, sentMessageID)
	}

	return nil
}

func (b *Bot) scheduleDelete(ctx context.Context, chatID, messageID int64) {
	go func() {
		timer := time.NewTimer(time.Duration(b.cfg.DeleteAfterSeconds) * time.Second)
		defer timer.Stop()

		<-timer.C

		// Webhook 请求返回后 r.Context 会被取消，删除任务需要脱离请求上下文。
		deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := b.client.deleteMessage(deleteCtx, chatID, messageID); err != nil {
			log.Printf("delete telegram message failed: %v", err)
		}
	}()
}

func (b *Bot) isAllowed(chatID, userID, senderChatID int64) bool {
	if len(b.allowedChats) == 0 && len(b.allowedUsers) == 0 {
		return true
	}

	if _, ok := b.allowedChats[chatID]; ok {
		return true
	}
	if senderChatID != 0 {
		if _, ok := b.allowedChats[senderChatID]; ok {
			return true
		}
	}
	if _, ok := b.allowedUsers[userID]; ok {
		return true
	}

	return false
}

func (b *Bot) recordLastMessage(ctx context.Context, message *telegramMessage) {
	text := strings.TrimSpace(extractMessageText(message))
	if text == "" {
		return
	}

	sender := resolveSender(message)
	key := userChatKey{
		SenderID: sender.KeyID,
		ChatID:   message.Chat.ID,
	}

	snapshot := messageSnapshot{
		ChatID:    message.Chat.ID,
		MessageID: message.MessageID,
		Text:      text,
	}

	b.mu.Lock()
	b.lastMessages[key] = snapshot
	b.mu.Unlock()

	if b.state != nil {
		data, err := json.Marshal(snapshot)
		if err == nil {
			if err := b.state.Set(ctx, stateLastMessageKey(message.Chat.ID, sender.KeyID), string(data), 24*time.Hour); err != nil {
				log.Printf("save telegram last message failed: %v", err)
			}
		}
	}
}

func (b *Bot) getLastMessage(ctx context.Context, chatID, senderID int64) (messageSnapshot, bool) {
	key := userChatKey{
		SenderID: senderID,
		ChatID:   chatID,
	}

	b.mu.RLock()
	value, ok := b.lastMessages[key]
	b.mu.RUnlock()
	if ok {
		return value, true
	}

	if b.state == nil {
		return messageSnapshot{}, false
	}

	data, err := b.state.Get(ctx, stateLastMessageKey(chatID, senderID))
	if err != nil || strings.TrimSpace(data) == "" {
		return messageSnapshot{}, false
	}

	var snapshot messageSnapshot
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		return messageSnapshot{}, false
	}

	b.mu.Lock()
	b.lastMessages[key] = snapshot
	b.mu.Unlock()

	return snapshot, true
}

func (b *Bot) toggleAutoTranslate(ctx context.Context, chatID, senderID int64) bool {
	key := userChatKey{
		SenderID: senderID,
		ChatID:   chatID,
	}

	current := b.isAutoTranslateEnabled(ctx, chatID, senderID)
	next := !current

	b.mu.Lock()
	b.autoTranslate[key] = next
	b.mu.Unlock()

	if b.state != nil {
		if err := b.state.Set(ctx, stateAutoTranslateKey(chatID, senderID), fmt.Sprintf("%t", next), 30*24*time.Hour); err != nil {
			log.Printf("save telegram auto translate state failed: %v", err)
		}
	}

	return next
}

func (b *Bot) isAutoTranslateEnabled(ctx context.Context, chatID, senderID int64) bool {
	key := userChatKey{
		SenderID: senderID,
		ChatID:   chatID,
	}

	b.mu.RLock()
	value, ok := b.autoTranslate[key]
	b.mu.RUnlock()
	if ok {
		return value
	}

	if b.state == nil {
		return false
	}

	data, err := b.state.Get(ctx, stateAutoTranslateKey(chatID, senderID))
	if err != nil {
		return false
	}

	enabled := strings.EqualFold(strings.TrimSpace(data), "true")
	b.mu.Lock()
	b.autoTranslate[key] = enabled
	b.mu.Unlock()

	return enabled
}

func stateLastMessageKey(chatID, senderID int64) string {
	return fmt.Sprintf("telegram:last_message:%d:%d", chatID, senderID)
}

func stateAutoTranslateKey(chatID, senderID int64) string {
	return fmt.Sprintf("telegram:auto_translate:%d:%d", chatID, senderID)
}

func detectTargetLanguage(text string) string {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return "EN"
		}
	}
	return "ZH"
}

func resolveSender(message *telegramMessage) senderContext {
	sender := senderContext{}
	if message == nil {
		return sender
	}

	sender.ChatID = message.Chat.ID

	if message.SenderChat != nil {
		sender.SenderChatID = message.SenderChat.ID
		sender.KeyID = message.SenderChat.ID
	}

	if message.From != nil {
		sender.UserID = message.From.ID
		if sender.KeyID == 0 {
			sender.KeyID = message.From.ID
		}
		if message.From.IsBot && message.SenderChat == nil {
			sender.Ignore = true
		}
	}

	if sender.KeyID == 0 {
		sender.KeyID = sender.ChatID
	}

	return sender
}

func formatSenderIDMessage(sender senderContext) string {
	parts := []string{"当前会话标识信息："}
	parts = append(parts, fmt.Sprintf("chat_id: %d", sender.ChatID))
	if sender.UserID != 0 {
		parts = append(parts, fmt.Sprintf("user_id: %d", sender.UserID))
	}
	if sender.SenderChatID != 0 {
		parts = append(parts, fmt.Sprintf("sender_chat_id: %d", sender.SenderChatID))
	}
	return strings.Join(parts, "\n")
}

func formatUnauthorizedMessage(sender senderContext) string {
	return strings.Join([]string{
		"⚠️ 当前用户或群组没有权限使用该机器人。",
		formatSenderIDMessage(sender),
		"请将以上 ID 提供给管理员加入白名单。",
	}, "\n")
}

func (b *Bot) replyUnauthorized(ctx context.Context, message *telegramMessage, sender senderContext) error {
	return b.replyText(ctx, message.Chat.ID, message.MessageID, formatUnauthorizedMessage(sender), false)
}

func extractMessageText(message *telegramMessage) string {
	if message == nil {
		return ""
	}
	if strings.TrimSpace(message.Text) != "" {
		return message.Text
	}
	return message.Caption
}

func parseSlashCommand(text, botUsername string) (command string, args string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return "", "", false
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "", "", false
	}

	token := strings.TrimPrefix(parts[0], "/")
	commandParts := strings.SplitN(token, "@", 2)
	command = strings.ToLower(strings.TrimSpace(commandParts[0]))
	if command == "" {
		return "", "", false
	}

	if len(commandParts) == 2 && botUsername != "" && !strings.EqualFold(commandParts[1], botUsername) {
		return "", "", false
	}

	args = strings.TrimSpace(strings.TrimPrefix(trimmed, parts[0]))
	return command, args, true
}

func isReplyTranslateTrigger(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return trimmed == "ts" || trimmed == "translate" || strings.TrimSpace(text) == "翻译"
}

func parseTextTranslateTrigger(text string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, ""
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return false, ""
	}

	first := strings.ToLower(parts[0])
	if first != "ts" && first != "translate" && parts[0] != "翻译" {
		return false, ""
	}

	if len(parts) == 1 {
		return true, ""
	}

	return true, strings.TrimSpace(strings.TrimPrefix(trimmed, parts[0]))
}

func startMessage() string {
	return strings.Join([]string{
		"欢迎使用 TransBridge Telegram Bot！",
		"",
		"支持的命令：",
		"/ts <text> - 翻译指定文本",
		"/translate <text> - 翻译指定文本",
		"/auto - 开启或关闭当前会话自动翻译",
		"/get_user_id - 获取你的用户ID",
		"/get_group_id - 获取当前群组ID",
		"",
		"快捷用法：",
		"1. 回复某条消息，发送 ts / translate / 翻译",
		"2. 直接发送 ts <文本> / 翻译 <文本>",
		"3. 在群里 @机器人后跟文本，也会触发翻译",
		"",
		"当前机器人不再转发请求到外部域名，而是直接使用本进程中的翻译配置。",
	}, "\n")
}

type replyOptions struct {
	ReplyToMessageID int64
	AutoDelete       bool
}

type apiClient struct {
	baseURL    string
	botToken   string
	httpClient *http.Client
}

func newAPIClient(baseURL, botToken string) *apiClient {
	return &apiClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		botToken: botToken,
		httpClient: &http.Client{
			Timeout: 70 * time.Second,
		},
	}
}

func (c *apiClient) getMe(ctx context.Context) (*telegramUser, error) {
	var response telegramAPIResponse[telegramUser]
	if err := c.call(ctx, "getMe", map[string]any{}, &response); err != nil {
		return nil, err
	}
	return &response.Result, nil
}

func (c *apiClient) getUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]telegramUpdate, error) {
	var response telegramAPIResponse[[]telegramUpdate]
	payload := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message"},
	}
	if err := c.call(ctx, "getUpdates", payload, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *apiClient) sendMessage(ctx context.Context, request sendMessageRequest) (int64, error) {
	var response telegramAPIResponse[telegramMessage]
	if err := c.call(ctx, "sendMessage", request, &response); err != nil {
		return 0, err
	}
	return response.Result.MessageID, nil
}

func (c *apiClient) deleteMessage(ctx context.Context, chatID, messageID int64) error {
	var response telegramAPIResponse[bool]
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.call(ctx, "deleteMessage", payload, &response)
}

func (c *apiClient) call(ctx context.Context, method string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.botToken, method),
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return err
	}

	switch result := output.(type) {
	case *telegramAPIResponse[telegramUser]:
		if !result.OK {
			return fmt.Errorf("telegram api error: %s", result.Description)
		}
	case *telegramAPIResponse[[]telegramUpdate]:
		if !result.OK {
			return fmt.Errorf("telegram api error: %s", result.Description)
		}
	case *telegramAPIResponse[telegramMessage]:
		if !result.OK {
			return fmt.Errorf("telegram api error: %s", result.Description)
		}
	case *telegramAPIResponse[bool]:
		if !result.OK {
			return fmt.Errorf("telegram api error: %s", result.Description)
		}
	}

	return nil
}

type telegramAPIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID      int64            `json:"message_id"`
	From           *telegramUser    `json:"from"`
	SenderChat     *telegramChat    `json:"sender_chat"`
	Chat           telegramChat     `json:"chat"`
	Text           string           `json:"text"`
	Caption        string           `json:"caption"`
	ReplyToMessage *telegramMessage `json:"reply_to_message"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	UserName  string `json:"username"`
}

type telegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type sendMessageRequest struct {
	ChatID              int64  `json:"chat_id"`
	Text                string `json:"text"`
	ReplyToMessageID    int64  `json:"reply_to_message_id,omitempty"`
	AllowWithoutReply   bool   `json:"allow_sending_without_reply,omitempty"`
	DisableNotification bool   `json:"disable_notification,omitempty"`
}
