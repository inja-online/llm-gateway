package main

import "embed"

// Embedded operator helpers + default subscription config (release binary).
// Install with: llm-gateway helpers install
//
//go:embed shell/*.sh
//go:embed assets/*
var embeddedFS embed.FS
