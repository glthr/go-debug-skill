// RPC client for headless Delve with optional logging.
package delvehelper

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-delve/delve/service/api"
	"github.com/go-delve/delve/service/rpc2"
)

type rpcLogger struct {
	*slog.Logger
	file *os.File // nil when output is discarded
}

func newRPCLogger() (*rpcLogger, error) {
	val := strings.TrimSpace(os.Getenv("DLV_RPC_LOG"))
	if val == "" {
		return &rpcLogger{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, nil
	}
	path := val
	if val == "1" || strings.EqualFold(val, "true") {
		path = filepath.Join(getDlvDir(), "rpc.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	l := &rpcLogger{
		Logger: slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})),
		file:   f,
	}
	l.Info("logging enabled", "path", path)
	return l, nil
}

func (l *rpcLogger) close() {
	if l.file != nil {
		l.Info("logging closed")
		_ = l.file.Close()
	}
}

type loggingClient struct {
	*rpc2.RPCClient
	log *rpcLogger
}

func summarizeState(state *api.DebuggerState) string {
	if state == nil {
		return "nil"
	}
	if state.Exited {
		return fmt.Sprintf("exited status=%d", state.ExitStatus)
	}
	if state.Running {
		return "running"
	}
	if state.SelectedGoroutine != nil {
		loc := &state.SelectedGoroutine.UserCurrentLoc
		if loc.File == "" {
			loc = &state.SelectedGoroutine.CurrentLoc
		}
		fn := "???"
		if loc.Function != nil {
			fn = loc.Function.Name()
		}
		return fmt.Sprintf("goroutine=%d %s:%d %s", state.SelectedGoroutine.ID, loc.File, loc.Line, fn)
	}
	return "stopped"
}

func (c *loggingClient) GetState() (*api.DebuggerState, error) {
	c.log.Debug("GetState")
	state, err := c.RPCClient.GetState()
	c.log.Debug("GetState result", "state", summarizeState(state), "err", err)
	return state, err
}

func (c *loggingClient) FindLocation(scope api.EvalScope, loc string, findInstructions bool, substitutePathRules [][2]string) ([]api.Location, string, error) {
	c.log.Debug("FindLocation", "loc", loc, "findInstructions", findInstructions)
	locs, s, err := c.RPCClient.FindLocation(scope, loc, findInstructions, substitutePathRules)
	c.log.Debug("FindLocation result", "locs", len(locs), "err", err)
	return locs, s, err
}

func (c *loggingClient) CreateBreakpoint(breakPoint *api.Breakpoint) (*api.Breakpoint, error) {
	c.log.Debug("CreateBreakpoint", "file", breakPoint.File, "line", breakPoint.Line, "addr", breakPoint.Addr)
	bp, err := c.RPCClient.CreateBreakpoint(breakPoint)
	if bp != nil {
		c.log.Debug("CreateBreakpoint result", "id", bp.ID, "file", bp.File, "line", bp.Line, "addr", bp.Addr, "err", err)
	} else {
		c.log.Debug("CreateBreakpoint result", "bp", nil, "err", err)
	}
	return bp, err
}

func (c *loggingClient) ListBreakpoints(all bool) ([]*api.Breakpoint, error) {
	c.log.Debug("ListBreakpoints", "all", all)
	bps, err := c.RPCClient.ListBreakpoints(all)
	c.log.Debug("ListBreakpoints result", "count", len(bps), "err", err)
	return bps, err
}

func (c *loggingClient) ClearBreakpoint(id int) (*api.Breakpoint, error) {
	c.log.Debug("ClearBreakpoint", "id", id)
	bp, err := c.RPCClient.ClearBreakpoint(id)
	if bp != nil {
		c.log.Debug("ClearBreakpoint result", "id", bp.ID, "file", bp.File, "line", bp.Line, "err", err)
	} else {
		c.log.Debug("ClearBreakpoint result", "bp", nil, "err", err)
	}
	return bp, err
}

func (c *loggingClient) Continue() <-chan *api.DebuggerState {
	c.log.Debug("Continue")
	ch := c.RPCClient.Continue()
	out := make(chan *api.DebuggerState, 1)
	go func() {
		state := <-ch
		c.log.Debug("Continue result", "state", summarizeState(state), "err", state.Err)
		out <- state
	}()
	return out
}

func (c *loggingClient) Next() (*api.DebuggerState, error) {
	c.log.Debug("Next")
	state, err := c.RPCClient.Next()
	c.log.Debug("Next result", "state", summarizeState(state), "err", err)
	return state, err
}

func (c *loggingClient) Step() (*api.DebuggerState, error) {
	c.log.Debug("Step")
	state, err := c.RPCClient.Step()
	c.log.Debug("Step result", "state", summarizeState(state), "err", err)
	return state, err
}

func (c *loggingClient) StepOut() (*api.DebuggerState, error) {
	c.log.Debug("StepOut")
	state, err := c.RPCClient.StepOut()
	c.log.Debug("StepOut result", "state", summarizeState(state), "err", err)
	return state, err
}

func (c *loggingClient) EvalVariable(scope api.EvalScope, expr string, cfg api.LoadConfig) (*api.Variable, error) {
	c.log.Debug("EvalVariable", "expr", expr)
	v, err := c.RPCClient.EvalVariable(scope, expr, cfg)
	if v != nil {
		c.log.Debug("EvalVariable result", "name", v.Name, "value", v.Value, "err", err)
	} else {
		c.log.Debug("EvalVariable result", "v", nil, "err", err)
	}
	return v, err
}

func (c *loggingClient) ListLocalVariables(scope api.EvalScope, cfg api.LoadConfig) ([]api.Variable, error) {
	c.log.Debug("ListLocalVariables")
	vars, err := c.RPCClient.ListLocalVariables(scope, cfg)
	c.log.Debug("ListLocalVariables result", "count", len(vars), "err", err)
	return vars, err
}

func (c *loggingClient) ListFunctionArgs(scope api.EvalScope, cfg api.LoadConfig) ([]api.Variable, error) {
	c.log.Debug("ListFunctionArgs")
	vars, err := c.RPCClient.ListFunctionArgs(scope, cfg)
	c.log.Debug("ListFunctionArgs result", "count", len(vars), "err", err)
	return vars, err
}

func (c *loggingClient) Stacktrace(goroutineID int64, depth int, opts api.StacktraceOptions, regs *api.LoadConfig) ([]api.Stackframe, error) {
	c.log.Debug("Stacktrace", "goroutineID", goroutineID, "depth", depth)
	frames, err := c.RPCClient.Stacktrace(goroutineID, depth, opts, regs)
	c.log.Debug("Stacktrace result", "count", len(frames), "err", err)
	return frames, err
}

func (c *loggingClient) ListGoroutines(start int, count int) ([]*api.Goroutine, int, error) {
	c.log.Debug("ListGoroutines", "start", start, "count", count)
	goroutines, next, err := c.RPCClient.ListGoroutines(start, count)
	c.log.Debug("ListGoroutines result", "count", len(goroutines), "next", next, "err", err)
	return goroutines, next, err
}

func (c *loggingClient) Disconnect(cont bool) error {
	c.log.Debug("Disconnect", "cont", cont)
	err := c.RPCClient.Disconnect(cont)
	c.log.Debug("Disconnect result", "err", err)
	c.log.close()
	return err
}

func getAddrFilePath() string {
	return filepath.Join(getDlvDir(), "addr")
}

func getAddr() (string, error) {
	if a := os.Getenv("DLV_ADDR"); a != "" {
		return a, nil
	}
	addrFile := getAddrFilePath()
	b, err := os.ReadFile(addrFile)
	if err != nil {
		return "", fmt.Errorf("no DLV_ADDR and %s not found: %w (start headless dlv and set DLV_ADDR or write address to %s)", addrFile, err, addrFile)
	}
	return strings.TrimSpace(string(b)), nil
}

func newClient() (*loggingClient, error) {
	addr, err := getAddr()
	if err != nil {
		return nil, err
	}
	log, err := newRPCLogger()
	if err != nil {
		return nil, err
	}
	log.Debug("NewClient", "addr", addr)
	return &loggingClient{RPCClient: rpc2.NewClient(addr), log: log}, nil
}

func scopeFromState(state *api.DebuggerState) api.EvalScope {
	if state.SelectedGoroutine != nil {
		return api.EvalScope{GoroutineID: state.SelectedGoroutine.ID, Frame: 0}
	}
	return api.EvalScope{GoroutineID: -1, Frame: 0}
}
