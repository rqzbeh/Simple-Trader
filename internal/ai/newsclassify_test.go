package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// T042/T019: structured-output contract — evidence→reasoning→label order.
func TestNewsClassifyContract(t *testing.T) {
	if idxE, idxR, idxL := indexOf(newsClassifySchema, "evidence"), indexOf(newsClassifySchema, "reasoning"), indexOf(newsClassifySchema, `"label"`); !(idxE < idxR && idxR < idxL) {
		t.Fatalf("schema field order must be evidence<reasoning<label, got %d %d %d", idxE, idxR, idxL)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{
				"content": "{\"evidence\":[\"ETF inflows record\"],\"reasoning\":\"flow\",\"label\":\"BULLISH\",\"confidence\":0.7}",
			}}},
		})
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{BaseURL: srv.URL, APIKey: "k", ModelID: "m"})
	res, err := c.ClassifyNews(context.Background(), "BTC", []string{"ETF inflows record"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Label != "BULLISH" || res.Confidence != 0.7 {
		t.Fatalf("bad result: %+v", res)
	}
	// Explicit errors
	if _, err := NewClient(ClientConfig{}).ClassifyNews(context.Background(), "BTC", []string{"h"}); err == nil {
		t.Fatal("unconfigured must error")
	}
	if _, err := c.ClassifyNews(context.Background(), "BTC", nil); err == nil {
		t.Fatal("empty cluster must error (FR-007)")
	}
}
