package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// runHelpers: llm-gateway helpers <install|list|path|print|source>
func runHelpers(args []string) error {
	if len(args) == 0 {
		printHelpersUsage()
		return fmt.Errorf("helpers: missing subcommand")
	}
	switch args[0] {
	case "install":
		return helpersInstall(args[1:])
	case "list":
		return helpersList()
	case "path":
		fmt.Println(helpersDefaultDir())
		return nil
	case "print", "cat":
		if len(args) < 2 {
			return fmt.Errorf("helpers print: need file name (e.g. claude-code-helpers.sh)")
		}
		return helpersPrint(args[1])
	case "source", "eval", "rc":
		return helpersPrintSource()
	case "help", "-h", "--help":
		printHelpersUsage()
		return nil
	default:
		printHelpersUsage()
		return fmt.Errorf("helpers: unknown subcommand %q", args[0])
	}
}

func printHelpersUsage() {
	fmt.Fprintf(os.Stderr, `Usage: llm-gateway helpers <command>

Install shell helpers (Claude Code, Cursor, multi-app) bundled in this binary.

Commands:
  install [--dir PATH]   Write helpers + subscription config to disk
  list                   List embedded helper files
  path                   Print default install directory
  print <file>           Print one embedded file to stdout
  source                 Print "source …" lines for your shell rc

Default install directory:
  $XDG_CONFIG_HOME/inja-gateway   (or ~/.config/inja-gateway)

After install:
  source ~/.config/inja-gateway/shell/claude-code-helpers.sh
  source ~/.config/inja-gateway/shell/cursor-helpers.sh
  source ~/.config/inja-gateway/shell/apps-helpers.sh
  export KEY=local-dev
  cc-gateway-up
  cc-gateway-logs -f
`)
}

func helpersDefaultDir() string {
	if d := os.Getenv("INJA_GATEWAY_HELPERS_DIR"); d != "" {
		return d
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "inja-gateway")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "inja-gateway")
	}
	return filepath.Join(home, ".config", "inja-gateway")
}

func helpersInstall(args []string) error {
	dir := helpersDefaultDir()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 >= len(args) {
				return fmt.Errorf("helpers install: --dir needs a path")
			}
			i++
			dir = args[i]
		case "-h", "--help":
			printHelpersUsage()
			return nil
		default:
			return fmt.Errorf("helpers install: unknown arg %q", args[i])
		}
	}
	shellDir := filepath.Join(dir, "shell")
	certsDir := filepath.Join(dir, "certs")
	if err := os.MkdirAll(shellDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		return err
	}

	// shell/*.sh
	entries, err := fs.ReadDir(embeddedFS, "shell")
	if err != nil {
		return fmt.Errorf("helpers: embedded shell: %w", err)
	}
	var written []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sh") {
			continue
		}
		data, err := embeddedFS.ReadFile("shell/" + name)
		if err != nil {
			return err
		}
		out := filepath.Join(shellDir, name)
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
		// mark primary helpers executable for convenience (still need source)
		_ = os.Chmod(out, 0o755)
		written = append(written, out)
	}

	// assets (subscription config)
	assetEntries, err := fs.ReadDir(embeddedFS, "assets")
	if err != nil {
		return fmt.Errorf("helpers: embedded assets: %w", err)
	}
	for _, e := range assetEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := embeddedFS.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, name)
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
		written = append(written, out)
	}

	fmt.Fprintf(os.Stderr, "Installed %d files under %s\n", len(written), dir)
	for _, w := range written {
		fmt.Fprintf(os.Stderr, "  %s\n", w)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Load in this shell (or add to ~/.zshrc / ~/.bashrc):")
	helpersPrintSourceTo(os.Stderr, dir)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Then:")
	fmt.Fprintln(os.Stderr, "  export KEY=local-dev")
	fmt.Fprintln(os.Stderr, "  llm-gateway auth login chatgpt   # and/or claude, grok")
	fmt.Fprintln(os.Stderr, "  cc-gateway-up")
	fmt.Fprintln(os.Stderr, "  cc-gateway-logs -f")
	return nil
}

func helpersList() error {
	return fs.WalkDir(embeddedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fmt.Println(path)
		return nil
	})
}

func helpersPrint(name string) error {
	// allow short names
	candidates := []string{name, "shell/" + name, "assets/" + name}
	for _, c := range candidates {
		data, err := embeddedFS.ReadFile(c)
		if err == nil {
			_, err = os.Stdout.Write(data)
			return err
		}
	}
	return fmt.Errorf("helpers: embedded file %q not found (try: llm-gateway helpers list)", name)
}

func helpersPrintSource() error {
	return helpersPrintSourceTo(os.Stdout, helpersDefaultDir())
}

func helpersPrintSourceTo(w io.Writer, dir string) error {
	shell := filepath.Join(dir, "shell")
	for _, f := range []string{
		"claude-code-helpers.sh",
		"cursor-helpers.sh",
		"apps-helpers.sh",
	} {
		fmt.Fprintf(w, "source %q\n", filepath.Join(shell, f))
	}
	return nil
}
