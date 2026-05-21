package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
)

const (
	launchdLabel = "com.cogria.cogriaclaw"
	systemdUnit  = "cogriaclaw.service"
)

// ServiceParams describes how the service should launch cogriaclaw. All paths
// must be absolute and on the internal disk (a LaunchAgent won't exec from an
// external/noowners volume).
type ServiceParams struct {
	BinPath    string // absolute path to the installed binary
	ConfigPath string // absolute path to config.yaml
	WorkDir    string // working directory
	LogPath    string // stdout/stderr log file (macOS); journald handles Linux
}

// RegisterService writes the service definition to disk but does NOT start it.
// Use StartService to launch.
func RegisterService(p ServiceParams) error {
	switch runtime.GOOS {
	case "darwin":
		return writeLaunchd(p)
	case "linux":
		return writeSystemd(p)
	default:
		return fmt.Errorf("unsupported OS %q (macOS and Linux only)", runtime.GOOS)
	}
}

// StartService starts the registered service and enables it at login/boot.
func StartService() error {
	switch runtime.GOOS {
	case "darwin":
		plist, err := launchdPlistPath()
		if err != nil {
			return err
		}
		_ = exec.Command("launchctl", "unload", plist).Run() // ignore: may not be loaded
		if out, err := exec.Command("launchctl", "load", "-w", plist).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl load: %v: %s", err, out)
		}
		return nil
	case "linux":
		if out, err := exec.Command("systemctl", "--user", "enable", "--now", systemdUnit).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl enable --now: %v: %s", err, out)
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

// StopService stops the service (and disables login/boot start).
func StopService() error {
	switch runtime.GOOS {
	case "darwin":
		plist, err := launchdPlistPath()
		if err != nil {
			return err
		}
		if out, err := exec.Command("launchctl", "unload", "-w", plist).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl unload: %v: %s", err, out)
		}
		return nil
	case "linux":
		if out, err := exec.Command("systemctl", "--user", "disable", "--now", systemdUnit).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl disable --now: %v: %s", err, out)
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

// RestartService restarts the installed native service via its manager.
func RestartService() error {
	switch runtime.GOOS {
	case "darwin":
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), launchdLabel)
		if out, err := exec.Command("launchctl", "kickstart", "-k", target).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl kickstart: %v: %s", err, out)
		}
		return nil
	case "linux":
		if out, err := exec.Command("systemctl", "--user", "restart", systemdUnit).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl restart: %v: %s", err, out)
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

// ServiceInstalled reports whether a service unit exists for cogriaclaw.
func ServiceInstalled() bool {
	path, err := unitPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Uninstall stops the service and removes its unit file.
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		plist, err := launchdPlistPath()
		if err != nil {
			return err
		}
		_ = exec.Command("launchctl", "unload", "-w", plist).Run()
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove plist: %w", err)
		}
		fmt.Printf("Removed LaunchAgent: %s\n", plist)
		return nil
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", systemdUnit).Run()
		unitPath, err := systemdUnitPath()
		if err != nil {
			return err
		}
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove unit: %w", err)
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		fmt.Printf("Removed systemd user unit: %s\n", unitPath)
		return nil
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func unitPath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return launchdPlistPath()
	case "linux":
		return systemdUnitPath()
	default:
		return "", fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func launchdPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func systemdUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdUnit), nil
}

func writeLaunchd(p ServiceParams) error {
	plist, err := launchdPlistPath()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
		<string>-config</string>
		<string>%s</string>
	</array>
	<key>WorkingDirectory</key><string>%s</string>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel, p.BinPath, p.ConfigPath, p.WorkDir, p.LogPath, p.LogPath)

	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	return os.WriteFile(plist, []byte(content), 0o644)
}

func writeSystemd(p ServiceParams) error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`[Unit]
Description=cogriaclaw — WhatsApp <-> LLM bridge
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -config %s
WorkingDirectory=%s
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, p.BinPath, p.ConfigPath, p.WorkDir)

	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

// LingerHint returns the command to enable boot-start without login on Linux,
// or "" on other systems.
func LingerHint() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	if u, err := user.Current(); err == nil {
		return "sudo loginctl enable-linger " + u.Username
	}
	return ""
}
