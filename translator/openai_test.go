package translator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestMarshalTranslationRequestIncludesStreamFalse(t *testing.T) {
	data, err := marshalTranslationRequest(openai.ChatCompletionRequest{
		Model: "test-model",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("marshalTranslationRequest() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	stream, ok := payload["stream"].(bool)
	if !ok || stream {
		t.Fatalf("payload[stream] = %v, want false", payload["stream"])
	}
}

func TestMarshalTranslationRequestDisablesThinking(t *testing.T) {
	data, err := marshalTranslationRequest(openai.ChatCompletionRequest{
		Model: "test-model",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("marshalTranslationRequest() error = %v", err)
	}

	var payload struct {
		Thinking struct {
			Type string `json:"type"`
		} `json:"thinking"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Thinking.Type != "disabled" {
		t.Fatalf("payload.thinking.type = %q, want disabled", payload.Thinking.Type)
	}
}

func TestSummarizeUpstreamBodyRedactsSensitiveFields(t *testing.T) {
	summary := summarizeUpstreamBody([]byte(`{"token":"secret","message":"empty choices"}`), 200)
	if strings.Contains(summary, "secret") {
		t.Fatalf("summary leaked sensitive value: %s", summary)
	}
	if !strings.Contains(summary, "<redacted>") {
		t.Fatalf("summary = %s, want redacted marker", summary)
	}
}
