// Package daemon provides nginx-style process control (a PID file + signals)
// and self-installation as a native service (launchd on macOS, systemd on
// Linux). No Windows support — cogriaclaw targets macOS and Linux servers.
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// PIDFile tracks the running instance so control commands (reload/stop/status)
// act on it instead of spawning duplicates.
type PIDFile struct {
	path string
}

func NewPIDFile(path string) *PIDFile { return &PIDFile{path: path} }

func (p *PIDFile) Path() string { return p.path }

// Acquire writes the current PID, refusing if another live instance holds the
// file. Caller should defer Release.
func (p *PIDFile) Acquire() error {
	if pid, ok := p.RunningPID(); ok {
		return fmt.Errorf("already running (pid %d) — use 'cogriaclaw stop' or 'cogriaclaw reload'", pid)
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("pidfile dir: %w", err)
	}
	return os.WriteFile(p.path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// Release removes the PID file but only if it still holds our PID (avoids a
// restarted instance deleting a newer one's file).
func (p *PIDFile) Release() {
	if data, err := os.ReadFile(p.path); err == nil {
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(data))); pid == os.Getpid() {
			os.Remove(p.path)
		}
	}
}

// RunningPID returns the PID recorded in the file if that process is alive.
func (p *PIDFile) RunningPID() (int, bool) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if !alive(pid) {
		return 0, false
	}
	return pid, true
}

// Signal sends sig to the running instance, erroring if none is running.
func (p *PIDFile) Signal(sig syscall.Signal) (int, error) {
	pid, ok := p.RunningPID()
	if !ok {
		return 0, fmt.Errorf("no running instance (no live pid in %s)", p.path)
	}
	if err := syscall.Kill(pid, sig); err != nil {
		return pid, fmt.Errorf("signal pid %d: %w", pid, err)
	}
	return pid, nil
}

// alive reports whether pid refers to a live process (signal 0 probe).
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
