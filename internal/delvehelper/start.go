// Start headless Delve and write address to .dlv/addr.
package delvehelper

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func findDlv() (string, error) {
	if path, err := exec.LookPath("dlv"); err == nil {
		return path, nil
	}
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return "", fmt.Errorf("dlv not in PATH and could not get GOPATH: %w", err)
	}
	gopath := strings.TrimSpace(string(out))
	if gopath == "" {
		return "", fmt.Errorf("dlv not in PATH and GOPATH is empty")
	}
	if idx := strings.IndexAny(gopath, ":;"); idx >= 0 {
		gopath = gopath[:idx]
	}
	path := filepath.Join(gopath, "bin", "dlv")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("dlv not in PATH and not found at %s: run 'make install-delve' or 'go install github.com/go-delve/delve/cmd/dlv@latest'", path)
	}
	return path, nil
}

func startDetached(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	testMode := fs.Bool("test", false, "run dlv test instead of dlv debug")
	execMode := fs.Bool("exec", false, "run dlv exec instead of dlv debug")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *testMode && *execMode {
		return fmt.Errorf("cannot use -test and -exec together")
	}
	target := "."
	if len(rest) > 0 {
		target = rest[0]
	}

	// Fix #5: if target is a directory with its own go.mod (separate module),
	// chdir into it and use "." so dlv debug runs in the right module context.
	// Remember the original CWD so we can also write .dlv/addr there, allowing
	// subsequent delve-helper commands from the caller's directory to find the session.
	origCWD, _ := os.Getwd()
	didChdir := false
	if target != "." && !*execMode {
		if _, err := os.Stat(filepath.Join(target, "go.mod")); err == nil {
			if err := os.Chdir(target); err != nil {
				return fmt.Errorf("chdir %s: %w", target, err)
			}
			target = "."
			didChdir = true
		}
	}

	dlvPath, err := findDlv()
	if err != nil {
		return err
	}
	debugBin := filepath.Join(os.TempDir(), "dlv-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	dlvArgs := []string{"--headless", "--accept-multiclient", "--api-version=2"}
	switch {
	case *execMode:
		dlvArgs = append(dlvArgs, "exec", target)
		if len(rest) > 1 {
			dlvArgs = append(dlvArgs, "--")
			dlvArgs = append(dlvArgs, rest[1:]...)
		}
	case *testMode:
		dlvArgs = append(dlvArgs, "test", "--output", debugBin, target)
		if len(rest) > 1 {
			dlvArgs = append(dlvArgs, rest[1:]...)
		}
	default:
		dlvArgs = append(dlvArgs, "debug", "--output", debugBin, target)
		if len(rest) > 1 {
			dlvArgs = append(dlvArgs, rest[1:]...)
		}
	}

	// Fix #1: use a temp file instead of a pipe for dlv's stdout.
	//
	// With a pipe, the goroutine draining it (`go io.Copy(io.Discard, stdout)`)
	// lives inside this process. When delve-helper exits after writing .dlv/addr,
	// that goroutine is destroyed, the read end of the pipe is closed, and dlv
	// gets SIGPIPE (signal 13) the next time it writes to stdout — killing the
	// debug session mid-session.
	//
	// A regular file never produces SIGPIPE: dlv's inherited fd remains valid
	// after this process exits, so dlv can write to it indefinitely.
	tmpOut, err := os.CreateTemp("", "dlv-stdout-*")
	if err != nil {
		return fmt.Errorf("create temp file for dlv stdout: %w", err)
	}
	tmpPath := tmpOut.Name()

	cmd := exec.Command(dlvPath, dlvArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = tmpOut

	if err := startDetached(cmd); err != nil {
		tmpOut.Close()
		os.Remove(tmpPath)
		return err
	}
	tmpOut.Close()           // close our write-side copy; dlv's inherited fd stays open
	defer os.Remove(tmpPath) // unlink after we've read the address

	// Poll the temp file until dlv writes "API server listening at: <addr>".
	tmpIn, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open dlv output file: %w", err)
	}
	defer tmpIn.Close()

	const prefix = "API server listening at: "
	var addr string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := tmpIn.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek dlv output: %w", err)
		}
		scanner := bufio.NewScanner(tmpIn)
		if scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, prefix) {
				addr = strings.TrimSpace(line[len(prefix):])
				break
			}
			if line != "" {
				return fmt.Errorf("unexpected dlv output: %s", line)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr == "" {
		return fmt.Errorf("timed out waiting for dlv to start")
	}

	// Write .dlv/addr and .dlv/pid. If DBG_DIR is set, use DBG_DIR/.dlv (so .dlv lives in the debug artifact dir).
	// When we chdired into a submodule, resolve the path from origCWD so the file is under project root.
	dlvDir := getDlvDir()
	if didChdir {
		dlvDir = filepath.Join(origCWD, dlvDir)
	}
	if err := os.MkdirAll(dlvDir, 0755); err != nil {
		return err
	}
	addrFile := filepath.Join(dlvDir, "addr")
	pidFile := filepath.Join(dlvDir, "pid")
	if err := os.WriteFile(addrFile, []byte(addr+"\n"), 0644); err != nil {
		return err
	}
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0644)
	// If we auto-chdired and DBG_DIR is not set, also write to the caller's cwd so subsequent commands find the session.
	if didChdir && os.Getenv("DBG_DIR") == "" {
		callerDlv := filepath.Join(origCWD, ".dlv")
		if err := os.MkdirAll(callerDlv, 0755); err == nil {
			os.WriteFile(filepath.Join(callerDlv, "addr"), []byte(addr+"\n"), 0644)
			os.WriteFile(filepath.Join(callerDlv, "pid"), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0644)
		}
	}
	fmt.Println("headless dlv started, address written to", addrFile)
	fmt.Println(addr)
	return nil
}

// cmdStop terminates a running Delve session started by cmdStart.
// Reads the PID from .dlv/pid (or DBG_DIR/.dlv/pid), sends SIGTERM, and removes the .dlv files.
func cmdStop() error {
	dlvDir := getDlvDir()
	pidFile := filepath.Join(dlvDir, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("no active delve session (pid file not found)")
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid pid in %s: %w", pidFile, err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("process %d not found; cleaning up\n", pid)
	} else {
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("signal: %v (process may have already exited)\n", err)
		} else {
			fmt.Printf("sent SIGTERM to delve (pid %d)\n", pid)
		}
	}
	os.Remove(filepath.Join(dlvDir, "addr"))
	os.Remove(pidFile)
	fmt.Println("session cleaned up")
	return nil
}
