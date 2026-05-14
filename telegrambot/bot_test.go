package telegrambot

import (
	"strings"
	"testing"
)

func TestDetectTargetLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "contains chinese", text: "你好 world", want: "EN"},
		{name: "english text", text: "hello world", want: "ZH"},
		{name: "japanese without chinese han", text: "こんにちは", want: "ZH"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := detectTargetLanguage(tt.text); got != tt.want {
				t.Fatalf("detectTargetLanguage(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseSlashCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		text        string
		botUsername string
		wantCommand string
		wantArgs    string
		wantOK      bool
	}{
		{name: "simple ts", text: "/ts hello", botUsername: "mybot", wantCommand: "ts", wantArgs: "hello", wantOK: true},
		{name: "command with bot suffix", text: "/translate@mybot hello world", botUsername: "mybot", wantCommand: "translate", wantArgs: "hello world", wantOK: true},
		{name: "other bot command", text: "/ts@otherbot hello", botUsername: "mybot", wantOK: false},
		{name: "not a command", text: "ts hello", botUsername: "mybot", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			command, args, ok := parseSlashCommand(tt.text, tt.botUsername)
			if ok != tt.wantOK {
				t.Fatalf("parseSlashCommand(%q) ok = %v, want %v", tt.text, ok, tt.wantOK)
			}
			if command != tt.wantCommand {
				t.Fatalf("parseSlashCommand(%q) command = %q, want %q", tt.text, command, tt.wantCommand)
			}
			if args != tt.wantArgs {
				t.Fatalf("parseSlashCommand(%q) args = %q, want %q", tt.text, args, tt.wantArgs)
			}
		})
	}
}

func TestParseTextTranslateTrigger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		wantOK  bool
		wantArg string
	}{
		{name: "ts prefix", text: "ts hello", wantOK: true, wantArg: "hello"},
		{name: "translate prefix", text: "translate hello", wantOK: true, wantArg: "hello"},
		{name: "chinese prefix", text: "翻译 你好", wantOK: true, wantArg: "你好"},
		{name: "exact ts", text: "ts", wantOK: true, wantArg: ""},
		{name: "normal text", text: "hello ts", wantOK: false, wantArg: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ok, arg := parseTextTranslateTrigger(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("parseTextTranslateTrigger(%q) ok = %v, want %v", tt.text, ok, tt.wantOK)
			}
			if arg != tt.wantArg {
				t.Fatalf("parseTextTranslateTrigger(%q) arg = %q, want %q", tt.text, arg, tt.wantArg)
			}
		})
	}
}

func TestResolveSender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		message      *telegramMessage
		wantKeyID    int64
		wantUserID   int64
		wantSenderID int64
		wantIgnore   bool
	}{
		{
			name: "normal user",
			message: &telegramMessage{
				Chat: telegramChat{ID: -1001},
				From: &telegramUser{ID: 123, IsBot: false},
			},
			wantKeyID:    123,
			wantUserID:   123,
			wantSenderID: 0,
			wantIgnore:   false,
		},
		{
			name: "normal bot should ignore",
			message: &telegramMessage{
				Chat: telegramChat{ID: -1001},
				From: &telegramUser{ID: 999, IsBot: true},
			},
			wantKeyID:    999,
			wantUserID:   999,
			wantSenderID: 0,
			wantIgnore:   true,
		},
		{
			name: "anonymous admin with sender chat",
			message: &telegramMessage{
				Chat:       telegramChat{ID: -2001},
				From:       &telegramUser{ID: 1087968824, IsBot: true},
				SenderChat: &telegramChat{ID: -2001},
			},
			wantKeyID:    -2001,
			wantUserID:   1087968824,
			wantSenderID: -2001,
			wantIgnore:   false,
		},
		{
			name: "sender chat only",
			message: &telegramMessage{
				Chat:       telegramChat{ID: -3001},
				SenderChat: &telegramChat{ID: -9001},
			},
			wantKeyID:    -9001,
			wantUserID:   0,
			wantSenderID: -9001,
			wantIgnore:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveSender(tt.message)
			if got.KeyID != tt.wantKeyID {
				t.Fatalf("resolveSender().KeyID = %d, want %d", got.KeyID, tt.wantKeyID)
			}
			if got.UserID != tt.wantUserID {
				t.Fatalf("resolveSender().UserID = %d, want %d", got.UserID, tt.wantUserID)
			}
			if got.SenderChatID != tt.wantSenderID {
				t.Fatalf("resolveSender().SenderChatID = %d, want %d", got.SenderChatID, tt.wantSenderID)
			}
			if got.Ignore != tt.wantIgnore {
				t.Fatalf("resolveSender().Ignore = %v, want %v", got.Ignore, tt.wantIgnore)
			}
		})
	}
}

func TestFormatUnauthorizedMessage(t *testing.T) {
	t.Parallel()

	message := formatUnauthorizedMessage(senderContext{
		ChatID:       -100123,
		UserID:       123456,
		SenderChatID: -100123,
	})

	for _, want := range []string{
		"没有权限",
		"chat_id: -100123",
		"user_id: 123456",
		"sender_chat_id: -100123",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("formatUnauthorizedMessage() missing %q in %q", want, message)
		}
	}
}
