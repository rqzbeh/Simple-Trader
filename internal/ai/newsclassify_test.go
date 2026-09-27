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

func TestNewsClassifyEvidenceShapes(t *testing.T) {
	// Shape 1: evidence as array of strings
	rawArray := []byte(`{"evidence":["ETF inflows record","Fed cut"],"reasoning":"macro tailwinds","label":"BULLISH","confidence":0.85}`)
	var resArr ClassifyNewsResult
	if err := json.Unmarshal(rawArray, &resArr); err != nil {
		t.Fatalf("unmarshal array evidence failed: %v", err)
	}
	if len(resArr.Evidence) != 2 || resArr.Evidence[0] != "ETF inflows record" || resArr.Evidence[1] != "Fed cut" {
		t.Fatalf("unexpected array evidence: %+v", resArr.Evidence)
	}

	// Shape 2: evidence as plain string
	rawStr := []byte(`{"evidence":"ETF inflows record single string","reasoning":"single headline","label":"BULLISH","confidence":0.75}`)
	var resStr ClassifyNewsResult
	if err := json.Unmarshal(rawStr, &resStr); err != nil {
		t.Fatalf("unmarshal string evidence failed: %v", err)
	}
	if len(resStr.Evidence) != 1 || resStr.Evidence[0] != "ETF inflows record single string" {
		t.Fatalf("unexpected string evidence: %+v", resStr.Evidence)
	}

	// Shape 3: evidence as null or empty string
	rawNull := []byte(`{"evidence":null,"reasoning":"no evidence","label":"NEUTRAL","confidence":0.5}`)
	var resNull ClassifyNewsResult
	if err := json.Unmarshal(rawNull, &resNull); err != nil {
		t.Fatalf("unmarshal null evidence failed: %v", err)
	}
	if len(resNull.Evidence) != 0 {
		t.Fatalf("expected empty evidence, got: %+v", resNull.Evidence)
	}
}
