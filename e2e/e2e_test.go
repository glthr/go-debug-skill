//go:build integration

// Package e2e_test runs the full debug-example workflow end to end:
//
//  1. Reset examples/ to initial state (make reset-examples).
//  2. Confirm the three expected test failures.
//  3. Run three scripted Delve sessions, each narrowing the investigation:
//     Session 1 — broad: breakpoint at Window() return; inspect final slice.
//     Session 2 — narrow: conditional breakpoint at clamp guard (start==12);
//                 step through mutation, confirm end=15 (bug).
//     Session 3 — verify: same conditional breakpoint after fix; confirm end=16.
//  4. Apply the fix to pipeline.go.
//  5. Verify all three tests pass.
//  6. Generate the PDF debug report via delve-helper report-build.
//  7. Assert the markdown report and PDF contain the expected content.
//
// Run with:
//
//	go test -v -tags integration -timeout 120s ./e2e/
package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// projectRoot walks upward from the test's working directory until it finds
// a directory containing both Makefile and go.mod (the go-debug project root).
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, errM := os.Stat(filepath.Join(dir, "Makefile"))
		_, errG := os.Stat(filepath.Join(dir, "go.mod"))
		if errM == nil && errG == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("project root not found: no Makefile+go.mod ancestor")
		}
		dir = parent
	}
}

// run executes a command in dir, fails the test on non-zero exit, and returns
// the combined stdout+stderr output.
func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Run(); err != nil {
		t.Fatalf("FAIL: %s %v (dir=%s)\n%s\n%v", name, args, dir, buf.String(), err)
	}
	return buf.String()
}

// runDelveStart runs "delve-helper start ." and returns the stdout output.
//
// Two fd-inheritance pitfalls must be avoided:
//
//  1. Do NOT set c.Stderr to a bytes.Buffer. delve-helper passes its own
//     os.Stderr to the dlv child (cmd.Stderr = os.Stderr). If that stderr is
//     a pipe connected to our buffer, dlv inherits the pipe and holds it open
//     indefinitely, making cmd.Wait() block forever.
//
//  2. Do NOT set c.Stderr = os.Stderr. dlv in --accept-multiclient mode stays
//     alive after the tracee exits (it waits for the next client). If it holds
//     the test process's real stderr fd, the test runner's WaitDelay fires and
//     the binary exits with "I/O incomplete" even though the test passed.
//
// Solution: redirect stderr to /dev/null. dlv inherits /dev/null, writes
// nothing useful there, and the fd causes no I/O pressure on the test runner.
func runDelveStart(t *testing.T, dir string) string {
	t.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s for writing: %v", os.DevNull, err)
	}
	defer devNull.Close()

	var stdout bytes.Buffer
	c := exec.Command("delve-helper", "start", ".")
	c.Dir = dir
	c.Stdout = &stdout
	c.Stderr = devNull // dlv inherits /dev/null — no pipe, no WaitDelay pressure
	if err := c.Run(); err != nil {
		t.Fatalf("delve-helper start failed (dir=%s): %v\nstdout: %s", dir, err, stdout.String())
	}
	return stdout.String()
}

// runLax runs a command and returns (combined output, error) without failing
// the test. Used for commands that may exit non-zero (e.g. "go test" on buggy
// code, or "delve-helper continue" after the tracee exits).
func runLax(dir, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.String(), err
}

// appendMD appends text to the markdown report file at path.
func appendMD(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("appendMD open %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatalf("appendMD write: %v", err)
	}
}

// assertReport fails the test for every string in checks that is absent from md.
func assertReport(t *testing.T, md string, checks []string) {
	t.Helper()
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("report missing expected content: %q", want)
		}
	}
}

// delveValue strips the "varname = " prefix from a Delve print output line,
// returning only the value portion. E.g. "end = 16" → "16".
func delveValue(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, " = "); i >= 0 {
		return strings.TrimSpace(s[i+3:])
	}
	return s
}

// filterArgs removes Delve's internal return-value variables (~r0, ~r1, …)
// from function args output so they don't clutter the report.
func filterArgs(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "~r") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// ── test ──────────────────────────────────────────────────────────────────────

func TestDebugExampleE2E(t *testing.T) {
	root := projectRoot(t)
	exampleDir := filepath.Join(root, "examples", "failing_test")
	date := time.Now().Format("2006-01-02")
	stamp := time.Now().Format("2006-01-02T15-04-05")

	// ── 1. Reset example/ to the buggy template ────────────────────────────────
	t.Log("1. make reset-examples")
	run(t, root, "make", "reset-examples")

	// ── 2. Baseline: three tests must fail ────────────────────────────────────
	t.Log("2. verify baseline failures")
	baseOut, _ := runLax(exampleDir, "go", "test", "./...", "-v")
	for _, name := range []string{
		"TestWindowLastFull",
		"TestWindowTrailing",
		"TestAggregateWindow6",
	} {
		if !strings.Contains(baseOut, "--- FAIL: "+name) {
			t.Fatalf("expected %s to fail in baseline run;\noutput:\n%s", name, baseOut)
		}
	}

	// ── 3. Create artifact directory and initialise the markdown report ────────
	t.Log("3. create artifact dir")
	dbgDir := filepath.Join(exampleDir, ".debug_e2e_"+stamp)
	if err := os.MkdirAll(dbgDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Remove Delve address file on cleanup so stale sessions don't affect later runs.
	t.Cleanup(func() { os.RemoveAll(filepath.Join(exampleDir, ".dlv")) })

	// Copy PDF/LaTeX templates from the shared template directory.
	shareDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "delve-debug")
	if entries, _ := os.ReadDir(shareDir); len(entries) > 0 {
		for _, e := range entries {
			if !e.IsDir() {
				b, _ := os.ReadFile(filepath.Join(shareDir, e.Name()))
				os.WriteFile(filepath.Join(dbgDir, e.Name()), b, 0644) //nolint:errcheck
			}
		}
	}

	reportMD := filepath.Join(dbgDir, "debug_report.md")
	if err := os.WriteFile(reportMD, nil, 0644); err != nil {
		t.Fatal(err)
	}

	// Write the report header, failing-tests output, and hypothesis now.
	// The Debugging Trace table is written AFTER all sessions complete so that
	// the complete 12-row table appears as a single unbroken markdown table
	// (not fragmented by intermediate evidence sections).
	appendMD(t, reportMD,
		"# Debug Report — Sensor Pipeline Window Bug\n\n"+
			"**Package:** example  **Date:** "+date+"\n\n"+
			"## Failing Tests\n\n"+
			"```text\n"+baseOut+"```\n\n"+
			"## Hypothesis\n\n"+
			"Test failures indicate Window() returns incorrectly-sized slices for windows\n"+
			"that extend to the end of the input slice. The filtered dataset has 16 readings;\n"+
			"with size=4, step=2, the last full window (start=12) should contain 4 elements.\n\n"+
			"Candidate bug: the upper-bound guard at `pipeline.go:27-28`.\n\n"+
			"Suspect code:\n\n"+
			"```go\n"+
			"if end > len(data)-1 {\n"+
			"    end = len(data) - 1\n"+
			"}\n"+
			"```\n\n"+
			"**Investigation plan:** (1) Broad pass — break at Window() return to confirm\n"+
			"truncated slices. (2) Narrow pass — conditional break at the guard for start=12.\n"+
			"(3) Verify fix.\n\n")

	// Accumulate trace rows and evidence blocks during sessions; write them all
	// in the correct order after all sessions complete (see Phase 10).
	var traceRows strings.Builder
	var evidence1, evidence2, fixVerify string

	// ── 4. Session 1: broad observation at Window() return ────────────────────
	// Set a breakpoint at the return statement (line 32) to inspect the fully
	// built windows slice. This confirms data loss before we know which
	// loop iteration causes it.
	t.Log("4. Session 1 — broad BP at pipeline.go:32 (return windows)")
	startOut := runDelveStart(t, exampleDir)
	if !strings.Contains(startOut, "headless dlv started") {
		t.Fatalf("Delve did not start:\n%s", startOut)
	}
	time.Sleep(300 * time.Millisecond)
	stateOut := run(t, exampleDir, "delve-helper", "state")
	if !strings.Contains(stateOut, "stopped") {
		t.Fatalf("unexpected Delve state after start:\n%s", stateOut)
	}

	run(t, exampleDir, "delve-helper", "break", "pipeline.go:32")
	traceRows.WriteString("| 1 | set | `pipeline.go:32` | Broad: inspect full windows slice at Window() return |\n")

	cont1Out := run(t, exampleDir, "delve-helper", "continue")
	if !strings.Contains(cont1Out, "pipeline.go:32") {
		t.Fatalf("expected BP hit at pipeline.go:32;\ncontinue output:\n%s", cont1Out)
	}
	traceRows.WriteString("| 2 | hit | `pipeline.go:32` | Window() about to return — inspect the slice |\n")

	lenWin := run(t, exampleDir, "delve-helper", "print", "len(windows)")
	win6Len := run(t, exampleDir, "delve-helper", "print", "len(windows[6])")
	win6e0 := run(t, exampleDir, "delve-helper", "print", "windows[6][0]")
	win6e1 := run(t, exampleDir, "delve-helper", "print", "windows[6][1]")
	win6e2 := run(t, exampleDir, "delve-helper", "print", "windows[6][2]")
	stack1 := run(t, exampleDir, "delve-helper", "stack")

	if !strings.Contains(lenWin, "8") {
		t.Errorf("expected len(windows)=8; got: %s", lenWin)
	}
	if !strings.Contains(win6Len, "3") {
		t.Errorf("expected len(windows[6])=3 (bug: last element dropped); got: %s", win6Len)
	}

	run(t, exampleDir, "delve-helper", "clear", "1")
	traceRows.WriteString("| 3 | clear | `pipeline.go:32` |" +
		" `windows[6]` has " + delveValue(win6Len) + " elements (want 4); data loss confirmed — refocus on loop body |\n")
	runLax(exampleDir, "delve-helper", "continue") // tracee exits; ignore error

	evidence1 = "\n---\n\n" +
		"## Evidence — Session 1: Broad Observation (`pipeline.go:32`)\n\n" +
		"### Go source context\n\n" +
		"```{.go highlightlines=32 firstnumber=30 highlightcolor=yellow!40}\n" +
		"    windows = append(windows, data[start:end])\n" +
		"}\n" +
		"return windows\n" +
		"```\n\n" +
		"### Variables at breakpoint\n\n" +
		"| Variable | Value | Note |\n" +
		"| --- | --- | --- |\n" +
		"| `len(windows)` | " + delveValue(lenWin) + " | correct window count |\n" +
		"| `len(windows[6])` | " + delveValue(win6Len) + " | **BUG** — expected 4, last element dropped |\n" +
		"| `windows[6][0]` | " + delveValue(win6e0) + " | |\n" +
		"| `windows[6][1]` | " + delveValue(win6e1) + " | |\n" +
		"| `windows[6][2]` | " + delveValue(win6e2) + " | last present element — `data[15]` is missing |\n\n" +
		"### Stack trace\n\n" +
		"```text\n" + stack1 + "```\n\n" +
		"**Finding:** `windows[6]` contains only **" + delveValue(win6Len) + " elements**" +
		" (expected 4). Data is being truncated somewhere in the loop body.\n" +
		"Narrowing to the clamp guard at `pipeline.go:27`.\n\n"

	// ── 5. Session 2: narrow — conditional BP at clamp guard (start == 12) ────
	// Use condition `start == 12` to hit only when Window() is building the
	// window that corresponds to the truncated windows[6].
	t.Log("5. Session 2 — conditional BP at pipeline.go:27 (start == 12)")
	runDelveStart(t, exampleDir)
	time.Sleep(300 * time.Millisecond)

	run(t, exampleDir, "delve-helper", "break", "pipeline.go:27 if start == 12")
	traceRows.WriteString("| 4 | set (conditional) | `pipeline.go:27` |" +
		" Clamp guard with start==12: target the window causing truncation |\n")

	cont2Out := run(t, exampleDir, "delve-helper", "continue")
	if !strings.Contains(cont2Out, "pipeline.go:27") {
		t.Fatalf("expected BP hit at pipeline.go:27;\ncontinue output:\n%s", cont2Out)
	}
	traceRows.WriteString("| 5 | hit | `pipeline.go:27` | Conditional fired: start=12, end=16 (pre-clamp) |\n")

	localsOut := run(t, exampleDir, "delve-helper", "locals")
	stackOut := run(t, exampleDir, "delve-helper", "stack")
	printSize := run(t, exampleDir, "delve-helper", "print", "size")
	printStep := run(t, exampleDir, "delve-helper", "print", "step")
	printStart := run(t, exampleDir, "delve-helper", "print", "start")
	printEnd := run(t, exampleDir, "delve-helper", "print", "end")
	printLenData := run(t, exampleDir, "delve-helper", "print", "len(data)")
	printLenData1 := run(t, exampleDir, "delve-helper", "print", "len(data)-1")

	if !strings.Contains(localsOut, "start = 12") {
		t.Errorf("expected start=12 in locals at hit; got:\n%s", localsOut)
	}
	if !strings.Contains(localsOut, "end = 16") {
		t.Errorf("expected end=16 in locals at hit (pre-clamp); got:\n%s", localsOut)
	}
	if !strings.Contains(printEnd, "16") {
		t.Errorf("expected print(end)=16 at breakpoint; got: %s", printEnd)
	}
	if !strings.Contains(printLenData, "16") {
		t.Errorf("expected print(len(data))=16; got: %s", printLenData)
	}
	if !strings.Contains(printLenData1, "15") {
		t.Errorf("expected print(len(data)-1)=15; got: %s", printLenData1)
	}

	// Step through the clamp body: line 27 (condition true) → 28 (assignment) → 30
	run(t, exampleDir, "delve-helper", "next")
	traceRows.WriteString("| 6 | next | `pipeline.go:28` | Condition `16 > 15` true — entered clamp body |\n")
	run(t, exampleDir, "delve-helper", "next")
	traceRows.WriteString("| 7 | next | `pipeline.go:30` | Assignment executed: `end = len(data)-1` |\n")

	printEndClamped := run(t, exampleDir, "delve-helper", "print", "end")
	if !strings.Contains(printEndClamped, "15") {
		t.Errorf("expected end=15 after clamp; got: %s", printEndClamped)
	}

	run(t, exampleDir, "delve-helper", "clear", "1")
	traceRows.WriteString("| 8 | clear | `pipeline.go:27` | Root cause confirmed: end=" +
		delveValue(printEndClamped) + " (should be 16) — applying fix |\n")
	runLax(exampleDir, "delve-helper", "continue") // tracee exits; ignore error

	evidence2 = "\n---\n\n" +
		"## Evidence — Session 2: Clamp Confirmation (`pipeline.go:27`, start==12)\n\n" +
		"### Go source context\n\n" +
		// Breakpoint line is 27; show lines 25-31 (firstnumber=25).
		"```{.go highlightlines=27 firstnumber=25 highlightcolor=yellow!40}\n" +
		"for start := 0; start < len(data); start += step {\n" +
		"    end := start + size\n" +
		"    if end > len(data)-1 {\n" +
		"        end = len(data) - 1\n" +
		"    }\n" +
		"    windows = append(windows, data[start:end])\n" +
		"}\n" +
		"```\n\n" +
		"### Variables at breakpoint (line 27, pre-clamp)\n\n" +
		"| Variable | Value | Note |\n" +
		"| --- | --- | --- |\n" +
		"| `size` | " + delveValue(printSize) + " | window size (arg) |\n" +
		"| `step` | " + delveValue(printStep) + " | window step (arg) |\n" +
		"| `len(data)` | " + delveValue(printLenData) + " | total filtered data points (arg) |\n" +
		"| `start` | " + delveValue(printStart) + " | current loop iteration — building `windows[6]` |\n" +
		"| `end` | " + delveValue(printEnd) + " | `start + size` = `12 + 4` = `16` (pre-clamp) |\n" +
		"| `len(data)-1` | " + delveValue(printLenData1) + " | buggy guard threshold — `end > 15` fires when `end = 16` |\n\n" +
		"**Condition check:** `end > len(data)-1` → `" +
		delveValue(printEnd) + " > " + delveValue(printLenData1) + "` → **TRUE** → enters clamp body\n\n" +
		"### After stepping through clamp (next ×2)\n\n" +
		"| Variable | Before | After | Note |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `end` | " + delveValue(printEnd) + " | **" + delveValue(printEndClamped) + "** |" +
		" incorrectly clamped — `data[15]` will be missing |\n\n" +
		"### Stack trace\n\n" +
		"```text\n" + stackOut + "```\n\n"

	// ── 6. Apply the fix ──────────────────────────────────────────────────────
	t.Log("6. apply fix")
	pipelinePath := filepath.Join(exampleDir, "pipeline.go")
	src, err := os.ReadFile(pipelinePath)
	if err != nil {
		t.Fatal(err)
	}
	buggy := "\t\tif end > len(data)-1 {\n" +
		"\t\t\tend = len(data) - 1"
	patched := "\t\tif end > len(data) {\n" +
		"\t\t\tend = len(data)"
	fixed := strings.Replace(string(src), buggy, patched, 1)
	if fixed == string(src) {
		t.Fatal("fix: bug string not found in pipeline.go — was example/ reset correctly?")
	}
	if err := os.WriteFile(pipelinePath, []byte(fixed), 0644); err != nil {
		t.Fatal(err)
	}

	// ── 7. Session 3: verify fix with the same conditional breakpoint ──────────
	// Restart Delve on the patched binary. The BP condition still fires
	// (start==12 is reached), but the clamp body is now skipped because
	// end > len(data) = 16 > 16 = false. Stepping once should land on line 30
	// with end unchanged at 16.
	t.Log("7. Session 3 — verify fix: same conditional BP, expect clamp to be skipped")
	runDelveStart(t, exampleDir)
	time.Sleep(300 * time.Millisecond)

	run(t, exampleDir, "delve-helper", "break", "pipeline.go:27 if start == 12")
	traceRows.WriteString("| 9 | set (conditional) | `pipeline.go:27` |" +
		" Verify fix: same condition, expect clamp body to be skipped |\n")

	run(t, exampleDir, "delve-helper", "continue")
	traceRows.WriteString("| 10 | hit | `pipeline.go:27` |" +
		" Conditional fired (start=12); checking whether clamp body is entered |\n")

	// Collect pre-next state: start, end, stack — all while paused at line 27.
	postStart := run(t, exampleDir, "delve-helper", "print", "start")
	postEndPre := run(t, exampleDir, "delve-helper", "print", "end")
	postStack := run(t, exampleDir, "delve-helper", "stack")

	// next: fixed condition (16 > 16) is false → jumps directly to line 30,
	// skipping the assignment body entirely.
	run(t, exampleDir, "delve-helper", "next")
	traceRows.WriteString("| 11 | next | `pipeline.go:30` |" +
		" Condition `16 > 16` false — clamp body skipped, end stays 16 |\n")

	postEnd := run(t, exampleDir, "delve-helper", "print", "end")
	if !strings.Contains(postEnd, "16") {
		t.Errorf("after fix: expected end=16 (clamp should not fire); got: %s", postEnd)
	}

	runLax(exampleDir, "delve-helper", "clear", "1")
	traceRows.WriteString("| 12 | clear | `pipeline.go:27` | Fix verified: end=" +
		delveValue(postEnd) + " confirmed — session complete |\n")
	runLax(exampleDir, "delve-helper", "continue")

	fixVerify = "## Fix Verification (Session 3 — Delve re-run)\n\n" +
		"Same conditional breakpoint (`pipeline.go:27 if start == 12`) on the patched binary.\n\n" +
		"### Variables at breakpoint (post-fix)\n\n" +
		"| Variable | Value | Note |\n" +
		"| --- | --- | --- |\n" +
		"| `start` | " + delveValue(postStart) + " | same iteration |\n" +
		"| `end` (at line 27) | " + delveValue(postEndPre) + " | `start + size` = `12 + 4` = `16` |\n\n" +
		"**Condition check:** `end > len(data)` → `" +
		delveValue(postEndPre) + " > 16` → **FALSE** → clamp body skipped\n\n" +
		"### After next (line 30, clamp body skipped)\n\n" +
		"| Variable | Before | After | Note |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `end` | " + delveValue(postEndPre) + " | **" + delveValue(postEnd) + "** | unchanged — fix confirmed |\n\n" +
		"### Stack trace\n\n" +
		"```text\n" + postStack + "```\n\n"

	// ── 8. Verify all tests pass ──────────────────────────────────────────────
	t.Log("8. verify tests pass")
	testOut := run(t, exampleDir, "go", "test", "./...", "-v")
	for _, name := range []string{
		"TestWindowLastFull",
		"TestWindowTrailing",
		"TestAggregateWindow6",
	} {
		if !strings.Contains(testOut, "--- PASS: "+name) {
			t.Errorf("expected %s to pass after fix;\noutput:\n%s", name, testOut)
		}
	}

	// ── 9. Write all accumulated sections to the report ───────────────────────
	// Writing the complete trace table in one block ensures Pandoc sees it as a
	// single unbroken table. Evidence sections follow in the correct order.
	t.Log("9. write report sections")
	appendMD(t, reportMD,
		"---\n\n"+
			"## Debugging Trace\n\n"+
			"| # | Action | Location | Reasoning |\n"+
			"| --- | --- | --- | --- |\n"+
			traceRows.String()+"\n")

	appendMD(t, reportMD, evidence1)
	appendMD(t, reportMD, evidence2)

	appendMD(t, reportMD,
		"## Root Cause\n\n"+
			"Window() at `pipeline.go:27-28` uses `len(data)-1` as the slice upper-bound guard.\n"+
			"Go slice syntax `s[low:high]` is **exclusive** at `high`; the valid maximum is\n"+
			"`len(s)`, not `len(s)-1`. The condition `end > len(data)-1` fires when\n"+
			"`end == len(data)`, clamping `end` to `len(data)-1` and dropping the last element\n"+
			"from every window that extends to end-of-slice.\n\n"+
			"**Exact divergence** at `start=12`, `end=16`, `len(data)=16`:\n\n"+
			"- Condition `16 > 15` fires (should not)\n"+
			"- `end` assigned to `15` (should remain `16`)\n"+
			"- `data[12:15]` → 3 elements (missing `data[15]`)\n\n"+
			"## Fix\n\n"+
			"```diff\n"+
			"- if end > len(data)-1 {\n"+
			"-     end = len(data) - 1\n"+
			"+ if end > len(data) {\n"+
			"+     end = len(data)\n"+
			"  }\n"+
			"```\n\n")

	appendMD(t, reportMD, fixVerify)

	appendMD(t, reportMD,
		"## Test Results After Fix\n\n"+
			"```text\n"+testOut+"```\n")

	// ── 10. Generate PDF ───────────────────────────────────────────────────────
	t.Log("10. generate PDF")
	// Let report-build derive the stamped filename from dbgDir automatically.
	run(t, exampleDir, "delve-helper", "report-build",
		"-pkg", "example",
		"-date", date,
		"-pdf",
		dbgDir,
	)
	pdfOut := filepath.Join(exampleDir, "debug_report_"+stamp+".pdf")

	// ── 11. Assess report ──────────────────────────────────────────────────────
	t.Log("11. assess report")

	fi, err := os.Stat(pdfOut)
	if err != nil {
		t.Fatalf("PDF not found at %s: %v", pdfOut, err)
	}
	const minPDFBytes = 50_000
	if fi.Size() < minPDFBytes {
		t.Errorf("PDF suspiciously small: %d bytes (want >= %d)", fi.Size(), minPDFBytes)
	}
	t.Logf("PDF: %s (%.0f KB)", pdfOut, float64(fi.Size())/1024)

	mdBytes, err := os.ReadFile(reportMD)
	if err != nil {
		t.Fatalf("cannot read %s: %v", reportMD, err)
	}
	assertReport(t, string(mdBytes), []string{
		// Structure: all required sections present.
		"## Hypothesis",
		"## Debugging Trace",
		"## Evidence — Session 1",
		"## Evidence — Session 2",
		"## Root Cause",
		"## Fix",
		"## Fix Verification",

		// Session 1: broad observation evidence.
		"pipeline.go:32",
		"len(windows)",

		// Session 2: runtime evidence (values appear in trace rows and variable tables).
		"end=16",      // pre-clamp — in trace row 5 "end=16 (pre-clamp)"
		"end=15",      // clamped  — in trace row 8 "end=15 (should be 16)"
		"start=12",    // loop index — in trace row 5 "start=12"
		"len(data)-1", // buggy expression — in hypothesis code block and evidence2 table

		// Session 2 and 3 breakpoint location.
		"pipeline.go:27",

		// Fix verification: condition check text.
		"16 > 16",

		// Test names must appear in the failing-tests output and results section.
		"TestWindowLastFull",
		"TestWindowTrailing",
		"TestAggregateWindow6",
	})

	t.Logf("DBG dir: %s", filepath.Base(dbgDir))
}
