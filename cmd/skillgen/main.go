// skillgen renders the canonical delve skill template into agent-specific skill files.
// Run from repo root: go run ./cmd/skillgen
// Optional: go run ./cmd/skillgen -out <dir> to write all skills under <dir> for review.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const sharedDescription = "Use when the user wants to debug a program, set breakpoints, step through code, inspect variables, or trace a bug in a running process. Examples: 'debug this program', 'set a breakpoint at line 42', 'why is this crashing', 'step through this function', 'inspect variable x', 'run with the debugger'. Detects language automatically; uses delve-helper for Go projects. During debugging the report is built in Markdown (.md) by delve-helper commands; only at the end is it converted to LaTeX and compiled to PDF."

const cursorDescription = "Use when the user wants to debug a program, set breakpoints, step through code, inspect variables, or investigate a crash, bug, test failure, or unexpected output. On ANY debug request (e.g. 'debug this program', 'debug this'): always verify project language first; if Go, systematically use delve-helper (steps 0–7). Examples: 'debug this program', 'debug this', 'run the debugger'. Also invoked with /delve. Report built in Markdown by delve-helper commands; at the end produce a LaTeX-formatted PDF using the tex templates, unless the user disables it (e.g. 'no PDF') or DELVE_SKIP_PDF is set."

type variant struct {
	OutPath        string
	Frontmatter    string
	Title          string
	PreActionGate  bool
	SlashCommand   bool
	TriggerConditions bool
	DebugModes     bool
	CommandReference bool
	SetupExtra     bool
}

func main() {
	outDir := flag.String("out", "", "optional: write all generated skills under this directory (for review); same layout: claude/delve.md, codex/delve/SKILL.md, cursor/delve-debug.mdc")
	flag.Parse()

	repoRoot := findRepoRoot()
	tmplPath := filepath.Join(repoRoot, "skills", "source", "delve.tmpl")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read template: %v\n", err)
		os.Exit(1)
	}
	t, err := template.New("delve").Parse(string(tmplBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse template: %v\n", err)
		os.Exit(1)
	}

	baseDir := filepath.Join(repoRoot, "skills")
	if *outDir != "" {
		baseDir = *outDir
	}

	variants := []variant{
		{
			OutPath:        filepath.Join(baseDir, "claude", "delve", "SKILL.md"),
			Frontmatter:    fmt.Sprintf("name: delve\ndescription: %q", sharedDescription),
			Title:          "Dynamic Debugger (delve-helper for Go)",
			PreActionGate:  true,
			SlashCommand:   false,
			TriggerConditions: false,
			DebugModes:     true,
			CommandReference: false,
			SetupExtra:     false,
		},
		{
			OutPath:        filepath.Join(baseDir, "codex", "delve", "SKILL.md"),
			Frontmatter:    fmt.Sprintf("name: delve\ndescription: %q", sharedDescription),
			Title:          "Dynamic Debugger (delve-helper for Go)",
			PreActionGate:  false,
			SlashCommand:   false,
			TriggerConditions: false,
			DebugModes:     true,
			CommandReference: false,
			SetupExtra:     false,
		},
		{
			OutPath:        filepath.Join(baseDir, "cursor", "delve-debug.mdc"),
			Frontmatter:    fmt.Sprintf("description: %q\nglobs: [\"**/*.go\", \"**/go.mod\", \"**/*.py\", \"**/*.js\", \"**/*.ts\", \"**/*.rs\", \"**/*.rb\", \"**/*.java\"]\nalwaysApply: true", cursorDescription),
			Title:          "Dynamic debugging (delve-helper for Go)",
			PreActionGate:  false,
			SlashCommand:   true,
			TriggerConditions: true,
			DebugModes:     false,
			CommandReference: true,
			SetupExtra:     true,
		},
	}

	for _, v := range variants {
		var buf bytes.Buffer
		if err := t.Execute(&buf, v); err != nil {
			fmt.Fprintf(os.Stderr, "execute template for %s: %v\n", v.OutPath, err)
			os.Exit(1)
		}
		// Ensure file ends with exactly one newline (required by many editors and parsers)
		b := bytes.TrimRight(buf.Bytes(), "\n")
		b = append(b, '\n')
		dir := filepath.Dir(v.OutPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
			os.Exit(1)
		}
		if err := os.WriteFile(v.OutPath, b, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", v.OutPath, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", v.OutPath)
	}
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintf(os.Stderr, "go.mod not found (run from repo root)\n")
			os.Exit(1)
		}
		dir = parent
	}
}
