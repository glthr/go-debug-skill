// Delve session commands: break, continue, print, locals, args, stack, etc.
package delvehelper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-delve/delve/service/api"
)

func printState(state *api.DebuggerState) error {
	if state.Exited {
		fmt.Printf("Process exited with status %d\n", state.ExitStatus)
		return nil
	}
	if state.Running {
		fmt.Println("Process is running.")
		return nil
	}
	printed := false
	if state.SelectedGoroutine != nil {
		loc := &state.SelectedGoroutine.UserCurrentLoc
		if loc.File == "" {
			loc = &state.SelectedGoroutine.CurrentLoc
		}
		fn := "???"
		if loc.Function != nil {
			fn = loc.Function.Name()
		}
		fmt.Printf("goroutine %d at %s:%d (%s)\n",
			state.SelectedGoroutine.ID, loc.File, loc.Line, fn)
		printed = true
	}
	for _, t := range state.Threads {
		if t.Breakpoint != nil {
			fmt.Printf("  thread %d at breakpoint %d: %s:%d\n",
				t.ID, t.Breakpoint.ID, t.File, t.Line)
			printed = true
		}
	}
	// Fix #4: always emit something so the agent knows the session is live.
	if !printed {
		fmt.Println("stopped")
	}
	return nil
}

func cmdBreak(client *loggingClient, state *api.DebuggerState, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: break <locspec> [if <condition>]")
	}
	locspec := strings.Join(args, " ")

	// Fix #2: parse optional "if <condition>" suffix before passing to FindLocation.
	// FindLocation only accepts a location spec; conditions are set on the Breakpoint struct.
	var cond string
	if idx := strings.Index(locspec, " if "); idx >= 0 {
		cond = strings.TrimSpace(locspec[idx+4:])
		locspec = strings.TrimSpace(locspec[:idx])
	}

	scope := scopeFromState(state)
	locs, _, err := client.FindLocation(scope, locspec, false, nil)
	if err != nil {
		return err
	}
	if len(locs) == 0 {
		return fmt.Errorf("no location found for %q", locspec)
	}
	for _, loc := range locs {
		addr := loc.PC
		if addr == 0 && len(loc.PCs) > 0 {
			addr = loc.PCs[0]
		}
		if addr == 0 {
			continue
		}
		bp := &api.Breakpoint{Addr: addr, File: loc.File, Line: loc.Line, Cond: cond}
		created, err := client.CreateBreakpoint(bp)
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("breakpoint %d at %s:%d (addr %#x)", created.ID, created.File, created.Line, created.Addr)
		if cond != "" {
			msg += fmt.Sprintf(" if %s", cond)
		}
		fmt.Println(msg)
	}
	return nil
}

func cmdBreakpoints(client *loggingClient) error {
	bps, err := client.ListBreakpoints(false)
	if err != nil {
		return err
	}
	for _, bp := range bps {
		if bp.ID == 0 {
			continue
		}
		dis := ""
		if bp.Disabled {
			dis = " (disabled)"
		}
		fmt.Printf("%d: %s:%d%s\n", bp.ID, bp.File, bp.Line, dis)
	}
	return nil
}

func cmdClear(client *loggingClient, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: clear <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	_, err = client.ClearBreakpoint(id)
	if err != nil {
		return err
	}
	fmt.Printf("cleared breakpoint %d\n", id)
	return nil
}

// isExitError reports whether err is Delve's "Process N has exited with status M"
// message. When the tracee exits during a continue/step, Delve sometimes delivers
// the exit via state.Err rather than state.Exited, so we need to handle both paths.
func isExitError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "has exited with status")
}

func cmdContinue(client *loggingClient) error {
	ch := client.Continue()
	state := <-ch
	if state.Err != nil {
		if isExitError(state.Err) {
			fmt.Println(state.Err)
			return nil
		}
		return state.Err
	}
	if state.Exited {
		fmt.Printf("Process exited with status %d\n", state.ExitStatus)
		return nil
	}
	return printState(state)
}

func cmdStep(client *loggingClient, name string) error {
	var state *api.DebuggerState
	var err error
	switch name {
	case api.Next:
		state, err = client.Next()
	case api.Step:
		state, err = client.Step()
	case api.StepOut:
		state, err = client.StepOut()
	default:
		return fmt.Errorf("unknown step command: %s", name)
	}
	if isExitError(err) {
		fmt.Println(err)
		return nil
	}
	if err != nil {
		return err
	}
	if state.Exited {
		fmt.Printf("Process exited with status %d\n", state.ExitStatus)
		return nil
	}
	return printState(state)
}

func cmdPrint(client *loggingClient, state *api.DebuggerState, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: print <expr>")
	}
	expr := strings.Join(args, " ")
	scope := scopeFromState(state)
	cfg := api.LoadConfig{FollowPointers: true, MaxVariableRecurse: 1, MaxStringLen: 200}
	v, err := client.EvalVariable(scope, expr, cfg)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("expression evaluated to nothing")
	}
	fmt.Printf("%s = %s\n", v.Name, v.Value)
	return nil
}

func cmdLocals(client *loggingClient, state *api.DebuggerState) error {
	scope := scopeFromState(state)
	cfg := api.LoadConfig{FollowPointers: true, MaxVariableRecurse: 1, MaxStringLen: 200}
	vars, err := client.ListLocalVariables(scope, cfg)
	if err != nil {
		return err
	}
	for _, v := range vars {
		fmt.Printf("%s = %s\n", v.Name, v.Value)
	}
	return nil
}

func cmdArgs(client *loggingClient, state *api.DebuggerState) error {
	scope := scopeFromState(state)
	cfg := api.LoadConfig{FollowPointers: true, MaxVariableRecurse: 1, MaxStringLen: 200}
	vars, err := client.ListFunctionArgs(scope, cfg)
	if err != nil {
		return err
	}
	for _, v := range vars {
		fmt.Printf("%s = %s\n", v.Name, v.Value)
	}
	return nil
}

func cmdStack(client *loggingClient, state *api.DebuggerState) error {
	goroutineID := int64(-1)
	if state.SelectedGoroutine != nil {
		goroutineID = state.SelectedGoroutine.ID
	}
	frames, err := client.Stacktrace(goroutineID, 20, 0, nil)
	if err != nil {
		return err
	}
	for i, f := range frames {
		fn := "???"
		if f.Function != nil {
			fn = f.Function.Name()
		}
		fmt.Printf("#%d %s %s:%d\n", i, fn, f.File, f.Line)
	}
	return nil
}

func cmdGoroutines(client *loggingClient) error {
	goroutines, _, err := client.ListGoroutines(0, 100)
	if err != nil {
		return err
	}
	for _, g := range goroutines {
		loc := &g.UserCurrentLoc
		if loc.File == "" {
			loc = &g.CurrentLoc
		}
		fn := "???"
		if loc.Function != nil {
			fn = loc.Function.Name()
		}
		fmt.Printf("goroutine %d [%s:%d %s]\n", g.ID, loc.File, loc.Line, fn)
	}
	return nil
}
