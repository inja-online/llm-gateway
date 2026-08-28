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
	gen := filepath.Join(dir, "scripts", "gen-localhost-tls.sh")
	st, err := os.Stat(gen)
	if err != nil || st.Size() < 100 {
		t.Fatalf("tls gen script: %v size=%v", err, st)
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
source ./shell/claude-code-profiles.sh
printf 'n=%s\n' "$(_inja_cc_normalize_providers grok)"
printf 'p=%s\n' "$(_inja_cc_normalize_providers gpt+grok)"
_inja_cc_apply_combo grok
printf 'm=%s\n' "$ANTHROPIC_MODEL"
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
	}
	if !gotOne {
		t.Skip("neither bash nor zsh on PATH")
	}
}

func TestEmbeddedTLSScriptMatchesExamples(t *testing.T) {
	ex, err := os.ReadFile(filepath.Join("..", "..", "examples", "scripts", "gen-localhost-tls.sh"))
	if err != nil {
		ex, err = os.ReadFile(filepath.Join("examples", "scripts", "gen-localhost-tls.sh"))
	}
	if err != nil {
		t.Skip("examples/scripts not found from test cwd")
	}
	emb, err := embeddedFS.ReadFile("scripts/gen-localhost-tls.sh")
	if err != nil {
		t.Fatal(err)
	}
	if string(ex) != string(emb) {
		t.Fatal("examples/scripts/gen-localhost-tls.sh and cmd/gateway/scripts differ — copy both when editing")
	}
}
