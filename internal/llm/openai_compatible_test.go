package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompleteJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "resp_123",
			"model": "deepseek-v4-flash",
			"choices": [
				{
					"message": {
						"content": "{\"answer\":\"ok\"}"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	var out struct {
		Answer string `json:"answer"`
	}

	meta, err := client.CompleteJSON(context.Background(), JSONRequest{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "system", Content: "return json"},
			{Role: "user", Content: "say ok"},
		},
		MaxTokens: 128,
	}, &out)
	if err != nil {
		t.Fatalf("CompleteJSON returned error: %v", err)
	}

	if out.Answer != "ok" {
		t.Fatalf("unexpected answer: %#v", out)
	}
	if meta.ID != "resp_123" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}
