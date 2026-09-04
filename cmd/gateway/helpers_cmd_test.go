package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpersListAndPrint(t *testing.T) {
	if err := helpersList(); err != nil {
		t.Fatal(err)
	}
	// print short name
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := helpersPrint("claude-code-helpers.sh")
	_ = w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if n == 0 || !strings.Contains(string(buf[:n]), "shellcheck") && !strings.Contains(string(buf[:n]), "Sourceable") {
		// file starts with shellcheck comment
		t.Fatalf("unexpected print head %q", string(buf[:n]))
	}
	if err := helpersPrint("does-not-exist-xyz.sh"); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestHelpersInstallAndSource(t *testing.T) {
	dir := t.TempDir()
	if err := helpersInstall([]string{"--dir", dir}); err != nil {
		t.Fatal(err)
	}
	// shell files
	for _, name := range []string{
		"claude-code-helpers.sh",
		"claude-code-profiles.sh",
		"cursor-helpers.sh",
		"apps-helpers.sh",
	} {
		p := filepath.Join(dir, "shell", name)
		st, err := os.Stat(p)
		if err != nil || st.Size() < 100 {
			t.Fatalf("%s: %v size=%v", p, err, st)
		}
	}
	cfg := filepath.Join(dir, "claude-code-subscriptions.yaml")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "certs")); err != nil {
		t.Fatal("certs dir", err)
	}
	for _, script := range []string{"gen-localhost-tls.sh", "claude-grok", "claude-gemini", "claude-codex"} {
		p := filepath.Join(dir, "scripts", script)
		st, err := os.Stat(p)
		if err != nil || st.Size() < 100 {
			t.Fatalf("%s: %v size=%v", p, err, st)
		}
		if st.Mode()&0o111 == 0 {
			t.Fatalf("%s: not executable", p)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "shell", "claude-code-helpers.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "$root/scripts/gen-localhost-tls.sh") {
		t.Fatal("installed helpers do not search scripts/gen-localhost-tls.sh")
	}

	// source lines point at install dir
	var b strings.Builder
	if err := helpersPrintSourceTo(&b, dir); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, filepath.Join(dir, "shell", "claude-code-helpers.sh")) {
		t.Fatalf("source lines: %s", out)
	}
}

func TestHelpersInstallBadArg(t *testing.T) {
	if err := helpersInstall([]string{"--nope"}); err == nil {
		t.Fatal("expected error")
	}
	if err := helpersInstall([]string{"--dir"}); err == nil {
		t.Fatal("expected --dir value error")
	}
}

func TestRunHelpersDispatch(t *testing.T) {
	if err := runHelpers(nil); err == nil {
		t.Fatal("expected missing subcommand")
	}
	if err := runHelpers([]string{"nope"}); err == nil {
		t.Fatal("expected unknown")
	}
	if err := runHelpers([]string{"help"}); err != nil {
		t.Fatal(err)
	}
	if err := runHelpers([]string{"path"}); err != nil {
		t.Fatal(err)
	}
	if err := runHelpers([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("INJA_GATEWAY_HELPERS_DIR", dir)
	if helpersDefaultDir() != dir {
		t.Fatalf("default dir %s", helpersDefaultDir())
	}
	if err := runHelpers([]string{"install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
}

func TestRunLoadHelpersAlias(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"load-helpers", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shell", "apps-helpers.sh")); err != nil {
		t.Fatal(err)
	}
}

func TestRunHelpersSubcommand(t *testing.T) {
	if err := run([]string{"helpers", "list"}); err != nil {
		t.Fatal(err)
	}
}

func TestHelpersDefaultDirXDG(t *testing.T) {
	t.Setenv("INJA_GATEWAY_HELPERS_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	d := helpersDefaultDir()
	if !strings.HasSuffix(d, filepath.Join("inja-gateway")) && !strings.Contains(d, "inja-gateway") {
		t.Fatalf("got %s", d)
	}
}

func TestEmbeddedShellMatchesExamples(t *testing.T) {
	// Keep repo examples/shell and embedded cmd/gateway/shell in sync.
	names := []string{
		"claude-code-helpers.sh",
		"claude-code-profiles.sh",
		"cursor-helpers.sh",
		"apps-helpers.sh",
	}
	for _, name := range names {
		ex, err := os.ReadFile(filepath.Join("..", "..", "examples", "shell", name))
		if err != nil {
			// running from module root vs package dir
			ex, err = os.ReadFile(filepath.Join("examples", "shell", name))
		}
		if err != nil {
			t.Skip("examples/shell not found from test cwd")
		}
		emb, err := embeddedFS.ReadFile("shell/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if string(ex) != string(emb) {
			t.Fatalf("%s: examples/shell and cmd/gateway/shell differ — copy both when editing", name)
		}
	}
}

func TestShellNormalizeProvidersZshAndBash(t *testing.T) {
	script := `
unset CC_MODEL CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_GROK_HEAVY CC_GEMINI_MID ANTHROPIC_MODEL
source ./shell/claude-code-profiles.sh
printf 'n=%s\n' "$(_inja_cc_normalize_providers grok)"
printf 'p=%s\n' "$(_inja_cc_normalize_providers gpt+grok)"
printf 'g=%s\n' "$(_inja_cc_normalize_providers gemini)"
_inja_cc_apply_combo grok
printf 'm=%s\n' "$ANTHROPIC_MODEL"
_inja_cc_apply_combo gemini
printf 'gm=%s\n' "$ANTHROPIC_MODEL"
`
	gotOne := false
	for _, sh := range []string{"bash", "zsh"} {
		path, err := exec.LookPath(sh)
		if err != nil {
			t.Logf("skip %s: not on PATH", sh)
			continue
		}
		gotOne = true
		cmd := exec.Command(path, "-c", script)
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", sh, err, out)
		}
		s := string(out)
		if !strings.Contains(s, "n=grok\n") {
			t.Fatalf("%s normalize grok: %q", sh, s)
		}
		if !strings.Contains(s, "p=gpt grok\n") {
			t.Fatalf("%s normalize gpt+grok: %q", sh, s)
		}
		if !strings.Contains(s, "m=grok-4.5\n") {
			t.Fatalf("%s apply grok model: %q", sh, s)
		}
		if !strings.Contains(s, "g=gemini\n") {
			t.Fatalf("%s normalize gemini: %q", sh, s)
		}
		if !strings.Contains(s, "gm=gemini\n") {
			t.Fatalf("%s apply gemini model: %q", sh, s)
		}
	}
	if !gotOne {
		t.Skip("neither bash nor zsh on PATH")
	}
}

func TestShellThinkingUltracode(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "ultracode-settings.json")
	script := `
unset CC_MODEL CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_GROK_HEAVY CC_GEMINI_MID ANTHROPIC_MODEL
unset CLAUDE_CODE_EFFORT_LEVEL
unset ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES
unset ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES
unset ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES
unset ANTHROPIC_CUSTOM_MODEL_OPTION ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES
source ./shell/claude-code-profiles.sh
_inja_cc_apply_combo grok
printf 'effort=%s\n' "$CLAUDE_CODE_EFFORT_LEVEL"
printf 'opus_caps=%s\n' "$ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES"
printf 'sonnet_caps=%s\n' "$ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES"
printf 'haiku_caps=%s\n' "$ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES"
printf 'custom=%s\n' "$ANTHROPIC_CUSTOM_MODEL_OPTION"
printf 'custom_caps=%s\n' "$ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES"
export CC_ULTRACODE_SETTINGS='` + settings + `'
f="$(_inja_cc_write_ultracode_settings)"
printf 'settings=%s\n' "$f"
_inja_cc_prepare_claude_launch
printf 'extra=%s\n' "${CC_LAUNCH_EXTRA[*]}"
export CLAUDE_CODE_EFFORT_LEVEL=ultracode
_inja_cc_apply_thinking
_inja_cc_prepare_claude_launch
printf 'coerced=%s\n' "${CC_LAUNCH_EXTRA[*]}"
`
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	cmd := exec.Command(path, "-c", script)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	s := string(out)
	if strings.Contains(s, "effort=xhigh\n") {
		t.Fatalf("must not default CLAUDE_CODE_EFFORT_LEVEL (pins /effort): %q", s)
	}
	for _, key := range []string{"opus_caps=", "sonnet_caps=", "haiku_caps=", "custom_caps="} {
		if !strings.Contains(s, key) || !strings.Contains(s, "xhigh_effort") {
			t.Fatalf("caps missing %s in %q", key, s)
		}
	}
	if !strings.Contains(s, "custom=grok-4.5\n") && !strings.Contains(s, "custom=grok-4.6\n") {
		t.Fatalf("custom option: %q", s)
	}
	if !strings.Contains(s, "--settings") {
		t.Fatalf("launch extra missing --settings: %q", s)
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "extra=") && strings.Contains(line, "--effort") {
			t.Fatalf("must not pass --effort unless env set: %q", s)
		}
	}
	if strings.Contains(s, "coerced=") && strings.Contains(s, "--effort ultracode") {
		t.Fatalf("ultracode must coerce to xhigh --effort: %q", s)
	}
	if !strings.Contains(s, "coerced=--effort xhigh") {
		t.Fatalf("coerced extra: %q", s)
	}
	body, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("settings file: %v\n%s", err, s)
	}
	js := string(body)
	for _, need := range []string{
		`"alwaysThinkingEnabled": true`,
		`"replaceBuiltInOptions": true`,
		`"behavesAs": "claude-fable-5"`,
		`"model": "grok-4.6"`,
		`"model": "gemini"`,
		`"model": "sol"`,
		`"model": "sonnet"`,
	} {
		if !strings.Contains(js, need) {
			t.Fatalf("settings missing %s:\n%s", need, js)
		}
	}
	for _, ban := range []string{`"effortLevel"`, `"ultracode": true`} {
		if strings.Contains(js, ban) {
			t.Fatalf("settings must not pin %s:\n%s", ban, js)
		}
	}
}

func TestAppsT3ClaudeWrappersMerge(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	client := filepath.Join(dir, "client-settings.json")
	if err := os.WriteFile(settings, []byte(`{
  "providerInstances": {
    "claudeAgent_claude_grok": {
      "driver": "claudeAgent",
      "config": {"binaryPath": "/tmp/claude-grok", "customModels": []}
    },
    "claudeAgent_claude_codex": {
      "driver": "claudeAgent",
      "config": {"binaryPath": "/tmp/claude-codex"}
    }
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(client, []byte(`{"providerModelPreferences":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
source ./shell/apps-helpers.sh
_apps_t3_settings_path() { printf '%s' '` + settings + `'; }
_apps_t3_client_settings_path() { printf '%s' '` + client + `'; }
apps-t3-claude-wrappers
`
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	cmd := exec.Command(path, "-c", script)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	body, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	js := string(body)
	if !strings.Contains(js, `"grok-4.6"`) || !strings.Contains(js, `"chatgpt/sol"`) {
		t.Fatalf("customModels not merged:\n%s\nout=%s", js, out)
	}
	cbody, err := os.ReadFile(client)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cbody), `"claude-opus-5"`) {
		t.Fatalf("hiddenModels not written:\n%s", cbody)
	}
}

func TestEmbeddedScriptsMatchExamples(t *testing.T) {
	names := []string{"gen-localhost-tls.sh", "claude-grok", "claude-gemini", "claude-codex"}
	for _, name := range names {
		ex, err := os.ReadFile(filepath.Join("..", "..", "examples", "scripts", name))
		if err != nil {
			ex, err = os.ReadFile(filepath.Join("examples", "scripts", name))
		}
		if err != nil {
			t.Skip("examples/scripts not found from test cwd")
		}
		emb, err := embeddedFS.ReadFile("scripts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if string(ex) != string(emb) {
			t.Fatalf("%s: examples/scripts and cmd/gateway/scripts differ — copy both when editing", name)
		}
	}
}
