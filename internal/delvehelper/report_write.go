// Report writing commands: report-init, report-hypothesis, report-trace-row,
// report-evidence, report-root-cause, report-fix, report-verification.
//
// The debug report is split across numbered .md files in the artifact dir so
// each section can be appended independently without collision:
//
//   00_report.md    – title + hypothesis
//   10_trace.md     – debugging trace table (rows appended incrementally)
//   20_evidence.md  – breakpoint evidence blocks (appended per stop)
//   90_conclusion.md – root cause + fix + post-fix verification
//
// report-build concatenates all .md files in sorted order, so the numbered
// scheme guarantees the correct section sequence in the final PDF.
//
// Agents MUST use these commands to manipulate the report; never write or
// edit report files directly.
package delvehelper

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	reportMainFile  = "00_report.md"
	reportTraceFile = "10_trace.md"
	reportEvidFile  = "20_evidence.md"
	reportConcFile = "90_conclusion.md"
)

func rfile(dir, name string) string { return filepath.Join(dir, name) }

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fileContains(path, s string) bool {
	b, _ := os.ReadFile(path)
	return strings.Contains(string(b), s)
}

// readSourceContext returns lines [line-ctx .. line+ctx] (1-indexed input) and
// the 1-based line number of the first returned line.
func readSourceContext(file string, line, ctx int) ([]string, int, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, 0, err
	}
	all := strings.Split(string(data), "\n")
	first := line - ctx - 1 // convert to 0-indexed
	if first < 0 {
		first = 0
	}
	last := line + ctx // exclusive, 0-indexed
	if last > len(all) {
		last = len(all)
	}
	return all[first:last], first + 1, nil
}

func fmtSourceBlock(lines []string, firstLine, highlightLine int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "```go {highlightlines=%d firstnumber=%d highlightcolor=yellow!40}\n",
		highlightLine, firstLine)
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	return sb.String()
}

// cmdReportInit creates the artifact dir, copies tex/lua templates, and
// writes 00_report.md with the report title header.
func cmdReportInit(args []string) error {
	fs := flag.NewFlagSet("report-init", flag.ContinueOnError)
	pkg := fs.String("pkg", "", "Go package name for report title")
	date := fs.String("date", "", "date YYYY-MM-DD (default: today)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-init [-pkg PKG] [-date DATE] <dbgdir>")
	}
	dir := fs.Arg(0)
	d := *date
	if d == "" {
		d = time.Now().Format("2006-01-02")
	}
	p := *pkg
	if p == "" {
		p = "unknown"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	// Copy tex and lua templates (needed by report-build / pdflatex).
	for _, sub := range []string{"templates/tex", "templates/lua"} {
		if err := installTemplatesDir(sub, dir); err != nil {
			return err
		}
	}
	header := fmt.Sprintf("# Debug Report — %s — %s\n\n", p, d)
	if err := os.WriteFile(rfile(dir, reportMainFile), []byte(header), 0644); err != nil {
		return fmt.Errorf("write %s: %w", reportMainFile, err)
	}
	fmt.Printf("initialized %s (pkg=%s date=%s)\n", dir, p, d)
	return nil
}

// cmdReportHypothesis appends the Hypothesis section to 00_report.md.
func cmdReportHypothesis(args []string) error {
	fs := flag.NewFlagSet("report-hypothesis", flag.ContinueOnError)
	loc := fs.String("loc", "file:line", "suspected location (file:line or func name)")
	expected := fs.String("expected", "<what should happen>", "expected behaviour")
	actual := fs.String("actual", "<what was observed>", "actual observed behaviour")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-hypothesis -loc LOC -expected TEXT -actual TEXT <dbgdir>")
	}
	dir := fs.Arg(0)
	section := fmt.Sprintf(
		"## Hypothesis\n\nSuspected location: `%s`\n\nExpected: %s\n\nActual: %s\n",
		*loc, *expected, *actual,
	)
	if err := appendToFile(rfile(dir, reportMainFile), "\n"+section); err != nil {
		return err
	}
	fmt.Println("appended hypothesis")
	return nil
}

// cmdReportTraceRow appends one row to the Debugging Trace table in 10_trace.md.
// Creates the file with section and table header on the first call.
func cmdReportTraceRow(args []string) error {
	fs := flag.NewFlagSet("report-trace-row", flag.ContinueOnError)
	n := fs.Int("n", 0, "row number")
	action := fs.String("action", "", "action: set | hit | clear | next | step")
	loc := fs.String("loc", "", "location (file:line or description)")
	reason := fs.String("reason", "", "one-line reasoning")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-trace-row -n N -action ACTION -loc LOC -reason REASON <dbgdir>")
	}
	dir := fs.Arg(0)
	path := rfile(dir, reportTraceFile)
	row := fmt.Sprintf("| %d | %s | `%s` | %s |\n", *n, *action, *loc, *reason)
	if !fileContains(path, "## Debugging Trace") {
		header := "## Debugging Trace\n\n| # | Action | Location | Reasoning |\n| - | ------ | -------- | --------- |\n"
		row = header + row
	}
	if err := appendToFile(path, row); err != nil {
		return err
	}
	fmt.Printf("appended trace row %d (%s)\n", *n, *action)
	return nil
}

// cmdReportEvidence appends one breakpoint evidence block to 20_evidence.md.
func cmdReportEvidence(args []string) error {
	fs := flag.NewFlagSet("report-evidence", flag.ContinueOnError)
	loc := fs.String("loc", "", "breakpoint location label (file:line)")
	srcFile := fs.String("src-file", "", "source file to read context from")
	highlight := fs.Int("highlight", 0, "line number to highlight")
	ctx := fs.Int("ctx", 2, "lines of context above and below highlight")
	argsOut := fs.String("args", "", "output of: delve-helper args")
	localsOut := fs.String("locals", "", "output of: delve-helper locals")
	stackOut := fs.String("stack", "", "output of: delve-helper stack")
	printExpr := fs.String("print-expr", "", "expression passed to delve-helper print")
	printVal := fs.String("print-val", "", "output of: delve-helper print <expr>")
	obs := fs.String("obs", "", "one-sentence observation (what was found)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-evidence -loc LOC [-src-file F -highlight N] " +
			"[-args A] [-locals L] [-stack S] [-print-expr E -print-val V] [-obs O] <dbgdir>")
	}
	dir := fs.Arg(0)
	path := rfile(dir, reportEvidFile)

	var sb strings.Builder
	if !fileContains(path, "## Breakpoints & Evidence") {
		sb.WriteString("## Breakpoints & Evidence\n")
	}
	sb.WriteString(fmt.Sprintf("\n### %s\n\n", *loc))

	if *srcFile != "" && *highlight > 0 {
		lines, firstLine, err := readSourceContext(*srcFile, *highlight, *ctx)
		if err == nil {
			sb.WriteString("**Source context:**\n\n")
			sb.WriteString(fmtSourceBlock(lines, firstLine, *highlight))
			sb.WriteString("\n")
		}
	}
	fmtBlock := func(label, text string) {
		if text == "" {
			return
		}
		sb.WriteString(fmt.Sprintf("**%s:**\n\n```text\n%s\n```\n\n",
			label, strings.TrimRight(text, "\n")))
	}
	fmtBlock("Args", *argsOut)
	fmtBlock("Locals", *localsOut)
	fmtBlock("Stack", *stackOut)
	if *printExpr != "" && *printVal != "" {
		sb.WriteString(fmt.Sprintf("**Print `%s`:**\n\n```text\n%s\n```\n\n",
			*printExpr, strings.TrimRight(*printVal, "\n")))
	} else if *printVal != "" {
		fmtBlock("Print", *printVal)
	}
	if *obs != "" {
		sb.WriteString(fmt.Sprintf("**Observation:** %s\n", *obs))
	}

	if err := appendToFile(path, sb.String()); err != nil {
		return err
	}
	fmt.Printf("appended evidence for %s\n", *loc)
	return nil
}

// cmdReportRootCause appends the Root Cause section to 90_conclusion.md.
func cmdReportRootCause(args []string) error {
	fs := flag.NewFlagSet("report-root-cause", flag.ContinueOnError)
	text := fs.String("text", "", "root cause description (one or two sentences)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-root-cause -text TEXT <dbgdir>")
	}
	dir := fs.Arg(0)
	section := fmt.Sprintf("## Root Cause\n\n%s\n", *text)
	if err := appendToFile(rfile(dir, reportConcFile), "\n"+section); err != nil {
		return err
	}
	fmt.Println("appended root cause")
	return nil
}

// cmdReportFix appends the Fix Applied section to 90_conclusion.md.
func cmdReportFix(args []string) error {
	fs := flag.NewFlagSet("report-fix", flag.ContinueOnError)
	text := fs.String("text", "", "fix description")
	diff := fs.String("diff", "", "unified diff of the change (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-fix -text TEXT [-diff DIFF] <dbgdir>")
	}
	dir := fs.Arg(0)
	var sb strings.Builder
	sb.WriteString("\n## Fix Applied\n\n")
	sb.WriteString(*text)
	sb.WriteString("\n")
	if *diff != "" {
		sb.WriteString(fmt.Sprintf("\n```diff\n%s\n```\n", strings.TrimRight(*diff, "\n")))
	}
	if err := appendToFile(rfile(dir, reportConcFile), sb.String()); err != nil {
		return err
	}
	fmt.Println("appended fix")
	return nil
}

// cmdReportVerification appends the Post-fix Verification section to 90_conclusion.md.
func cmdReportVerification(args []string) error {
	fs := flag.NewFlagSet("report-verification", flag.ContinueOnError)
	text := fs.String("text", "", "verification result (what was confirmed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: report-verification -text TEXT <dbgdir>")
	}
	dir := fs.Arg(0)
	section := fmt.Sprintf("\n## Post-fix Verification\n\n%s\n", *text)
	if err := appendToFile(rfile(dir, reportConcFile), section); err != nil {
		return err
	}
	fmt.Println("appended verification")
	return nil
}

