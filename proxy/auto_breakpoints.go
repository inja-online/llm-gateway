package proxy

import (
	"strings"

	"github.com/inja-online/llm-gateway/canonical"
	"github.com/inja-online/llm-gateway/config"
)

// applyAutoBreakpoints optionally inserts Anthropic-style ephemeral
// cache_control on translate → Anthropic paths when
// caching.auto_breakpoints.enabled is true.
//
// Rules (v1):
//   - Opt-in only (default off).
//   - Targets: system (last text system block) and/or tools (last tool).
//   - Client wins: if any cache_control already exists on that target surface, skip it.
//   - min_chars: total text length for the target must meet the threshold.
//   - Never invents Google cachedContent or OpenAI prompt_cache_*.
//
// Returns the list of targets that received a breakpoint (for X-Gateway-Cache-Auto).
func applyAutoBreakpoints(cfg *config.Config, req *canonical.Request) []string {
	if cfg == nil || req == nil || !cfg.Caching.AutoBreakpoints.Enabled {
		return nil
	}
	ab := cfg.Caching.AutoBreakpoints
	minChars := ab.AutoBreakpointMinChars()
	var applied []string
	for _, t := range ab.AutoBreakpointTargets() {
		switch t {
		case "system":
			if autoBreakpointSystem(req, minChars) {
				applied = append(applied, "system")
			}
		case "tools":
			if autoBreakpointTools(req, minChars) {
				applied = append(applied, "tools")
			}
		}
	}
	return applied
}

func autoBreakpointSystem(req *canonical.Request, minChars int) bool {
	if len(req.System) == 0 {
		return false
	}
	// Client wins: any existing breakpoint on system → skip.
	for _, b := range req.System {
		if b.CacheControl != nil {
			return false
		}
	}
	var n int
	lastText := -1
	for i, b := range req.System {
		if b.Type == canonical.BlockText || b.Type == "" {
			n += len(b.Text)
			lastText = i
		}
	}
	if lastText < 0 || n < minChars {
		return false
	}
	req.System[lastText].CacheControl = &canonical.CacheControl{Type: "ephemeral"}
	return true
}

func autoBreakpointTools(req *canonical.Request, minChars int) bool {
	if len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		if t.CacheControl != nil {
			return false
		}
	}
	var n int
	for _, t := range req.Tools {
		n += len(t.Name) + len(t.Description) + len(t.Schema)
	}
	if n < minChars {
		return false
	}
	last := len(req.Tools) - 1
	req.Tools[last].CacheControl = &canonical.CacheControl{Type: "ephemeral"}
	return true
}

// setCacheAutoHeader records which auto-breakpoint targets were applied.
func (x *exchange) setCacheAutoHeader(targets []string) {
	if x == nil || x.w == nil || len(targets) == 0 {
		return
	}
	x.w.Header().Set("X-Gateway-Cache-Auto", strings.Join(targets, ","))
}
