package proxy

import (
	"encoding/json"
	"strings"
)

// noteDroppedFields records translation drop names when observe_dropped_fields
// is enabled: response header x-gateway-dropped-fields (names only) + usage event.
func (x *exchange) noteDroppedFields(fields []string) {
	if x == nil || x.s == nil || x.s.cfg == nil || !x.s.cfg.ObserveDroppedFields {
		return
	}
	if len(fields) == 0 {
		return
	}
	// Dedupe while preserving order.
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	if len(out) == 0 {
		return
	}
	x.w.Header().Set("X-Gateway-Dropped-Fields", strings.Join(out, ","))
	x.ev.DroppedFields = out
}

// openaiTranslateDrops lists OpenAI request keys that are not rebuilt on
// cross-dialect egress (see docs/deprecation-policy.md + common_drops.txt).
func openaiTranslateDrops(body []byte) []string {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return nil
	}
	var drops []string
	keys := []string{
		"logprobs", "top_logprobs", "logit_bias", "stream_options",
		"service_tier", "prediction", "modalities", "user",
		"parallel_tool_calls", "store",
	}
	for _, k := range keys {
		if _, ok := m[k]; ok {
			drops = append(drops, "openai."+k)
		}
	}
	if n, ok := m["n"].(float64); ok && n > 1 {
		drops = append(drops, "openai.n")
	}
	return drops
}

// anthropicTranslateDrops lists Anthropic-only wire features stripped on translate.
func anthropicTranslateDrops(body []byte) []string {
	s := string(body)
	var drops []string
	if strings.Contains(s, "cache_control") {
		drops = append(drops, "anthropic.cache_control")
	}
	return drops
}
