// Run is the CLI entry point, called by cmd/delve-helper/main.go.
package delvehelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-delve/delve/service/api"
)

// getDlvDir returns the directory for .dlv/addr and .dlv/pid, relative to cwd.
// If DBG_DIR is set (e.g. .debug_2025-02-28), .dlv is created inside it so the
// project root stays clean. Otherwise .dlv is created in the current directory.
func getDlvDir() string {
	if d := os.Getenv("DBG_DIR"); d != "" {
		return filepath.Join(d, ".dlv")
	}
	return ".dlv"
}

// Run dispatches CLI arguments to the appropriate command handler.
func Run(argv []string) error {
	if len(argv) < 2 {
		printUsage()
		return nil
	}
	cmd := strings.ToLower(argv[1])
	args := argv[2:]

	if cmd == "start" {
		return cmdStart(args)
	}
	if cmd == "stop" {
		return cmdStop()
	}
	if cmd == "install-templates" {
		return cmdInstallTemplates()
	}
	if cmd == "report-build" {
		return cmdReportBuild(args)
	}
	if cmd == "report-init" {
		return cmdReportInit(args)
	}
	if cmd == "report-hypothesis" {
		return cmdReportHypothesis(args)
	}
	if cmd == "report-trace-row" {
		return cmdReportTraceRow(args)
	}
	if cmd == "report-evidence" {
		return cmdReportEvidence(args)
	}
	if cmd == "report-root-cause" {
		return cmdReportRootCause(args)
	}
	if cmd == "report-fix" {
		return cmdReportFix(args)
	}
	if cmd == "report-verification" {
		return cmdReportVerification(args)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Disconnect(false)

	state, err := client.GetState()
	if err != nil {
		// Fix #3: when the tracee has already exited, GetState returns an error
		// like "Process N has exited with status M". Treat this as informational
		// (exit 0) rather than a hard failure so the agent sees a clean message.
		if strings.Contains(err.Error(), "has exited with status") {
			fmt.Println(err)
			return nil
		}
		return err
	}

	switch cmd {
	case "state":
		return printState(state)
	case "break":
		return cmdBreak(client, state, args)
	case "breakpoints", "bp":
		return cmdBreakpoints(client)
	case "clear":
		return cmdClear(client, args)
	case "continue", "c":
		return cmdContinue(client)
	case "next", "n":
		return cmdStep(client, api.Next)
	case "step", "s":
		return cmdStep(client, api.Step)
	case "stepout", "so":
		return cmdStep(client, api.StepOut)
	case "print", "p":
		return cmdPrint(client, state, args)
	case "locals":
		return cmdLocals(client, state)
	case "args":
		return cmdArgs(client, state)
	case "stack", "bt":
		return cmdStack(client, state)
	case "goroutines", "grs":
		return cmdGoroutines(client)
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: delve-helper <command> [args]

Session lifecycle:
  start [-test|-exec] [pkg|binary]  Start headless dlv. Writes addr and pid to DBG_DIR/.dlv/ if DBG_DIR is set, else .dlv/.
  stop               Terminate the running Delve session (SIGTERM) and clean up .dlv/.
  state              Print current debugger state.

Breakpoint & execution control:
  break <locspec> [if <cond>]  Set breakpoint (e.g. main.go:42, main.main, "main.go:55 if x==5").
  breakpoints        List all breakpoints.
  clear <id>         Clear breakpoint by ID.
  continue           Resume execution until next stop.
  next               Step over.
  step               Step into.
  stepout            Step out of current function.

Inspection:
  print <expr>       Evaluate expression.
  locals             Print local variables.
  args               Print function arguments.
  stack              Print stack trace.
  goroutines         List goroutines.

Report writing (use these; never edit report files directly):
  report-init [-pkg PKG] [-date DATE] <dir>
                     Create artifact dir, copy templates, init 00_report.md.
  report-hypothesis -loc LOC -expected TEXT -actual TEXT <dir>
                     Append Hypothesis section to 00_report.md.
  report-trace-row -n N -action ACTION -loc LOC -reason REASON <dir>
                     Append one row to Debugging Trace table (10_trace.md).
  report-evidence -loc LOC [-src-file F -highlight N] [-args A] [-locals L]
                  [-stack S] [-print-expr E -print-val V] [-obs O] <dir>
                     Append breakpoint evidence block (20_evidence.md).
  report-root-cause -text TEXT <dir>
                     Append Root Cause section (90_conclusion.md).
  report-fix -text TEXT [-diff DIFF] <dir>
                     Append Fix Applied section (90_conclusion.md).
  report-verification -text TEXT <dir>
                     Append Post-fix Verification section (90_conclusion.md).
  report-build [-pkg pkg] [-date date] [-pdf] [-out path] [-v] <dir>
                     Convert all .md files → LaTeX; -pdf compiles to PDF.

Templates:
  install-templates  Extract embedded LaTeX/Lua templates to ~/.local/share/delve-debug/.

Logging: set DLV_RPC_LOG=1 (logs to .dlv/rpc.log) or DLV_RPC_LOG=/path/to/log.
When DBG_DIR is set (e.g. .debug_YYYY-MM-DD), .dlv is created inside it so the project root stays clean.
`)
}
