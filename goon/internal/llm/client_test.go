package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"goon/internal/config"
)

func TestChatCompletion_RequestResponse(t *testing.T) {
	t.Setenv("GOON_TEST_KEY", "secret-xyz")
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"## 目标\nok"}}]}`)
	}))
	defer srv.Close()

	c := New(config.LLM{Provider: "openai-compatible", BaseURL: srv.URL, APIKeyEnv: "GOON_TEST_KEY", Model: "gpt-4o-mini"})
	out, err := c.Complete("hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "## 目标\nok" {
		t.Fatalf("bad completion: %q", out)
	}
	if gotAuth != "Bearer secret-xyz" {
		t.Fatalf("key should come from env, auth=%q", gotAuth)
	}
	if gotBody["model"] != "gpt-4o-mini" {
		t.Fatalf("model not sent: %v", gotBody["model"])
	}
}
