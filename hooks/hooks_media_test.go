package hooks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUsageEventMediaJSON(t *testing.T) {
	ev := UsageEvent{
		RequestID: "req_1",
		Time:      time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		DialectIn: "openai",
		Provider:  "openai",
		Model:     "dall-e-3",
		Modality:  "image_gen",
		Transport: TransportHTTP,
		Estimated: true,
		Media: &MediaUsage{
			Units:    2,
			UnitKind: MediaUnitImage,
			Size:     "1024x1024",
		},
		Status:     StatusOK,
		HTTPStatus: 200,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["modality"] != "image_gen" {
		t.Fatalf("modality: %v", m["modality"])
	}
	media, ok := m["media"].(map[string]any)
	if !ok {
		t.Fatalf("media missing: %s", b)
	}
	if media["units"] != float64(2) || media["unit_kind"] != "image" {
		t.Fatalf("media: %v", media)
	}
}

func TestUsageEventOmitsEmptyMedia(t *testing.T) {
	ev := UsageEvent{RequestID: "r", Status: StatusOK}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "" && containsJSONKey(string(b), "media") {
		t.Fatalf("empty media should omit: %s", b)
	}
}

func containsJSONKey(s, key string) bool {
	return len(s) > 0 && (json.Valid([]byte(s)) &&
		// crude but enough for this test
		(len(key) > 0 && (containsStr(s, `"`+key+`":`) || containsStr(s, `"`+key+`": `))))
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
