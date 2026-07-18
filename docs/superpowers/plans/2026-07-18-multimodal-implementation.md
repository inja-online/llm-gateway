# Multimodal Gateway — Implementation Plan

Spec: `docs/superpowers/specs/2026-07-18-multimodal-gateway-design.md`  
Branch: `feat/multimodal-gateway`  
Constraints: TDD, air-gapped tests, coverage ≥90%, no real provider calls.

## PR0 — Core skeleton
- [ ] Modalities + transport constants
- [ ] `config.Capabilities` + defaults by kind + `Supports(modality)`
- [ ] `realtime` config section
- [ ] `hooks.UsageEvent` modality/transport/media
- [ ] `proxy.Resolve` + `CheckCapability` / `ResolveForModality`
- [ ] Wire capability deny into existing image/video handlers
- [ ] `internal/testutil` + `testdata/fixtures/README.md`
- [ ] Update example YAML + failing media tests for opt-in caps

## PR1 — Image gen general
- Anthropic/Google ingress routes + canonical image + translation

## PR2 — Video gen general
## PR3 — Voice HTTP
## PR4 — Chat audio input
## PR5 — Realtime passthrough
## PR6 — Realtime bridge
## PR7 — Hardening/docs
