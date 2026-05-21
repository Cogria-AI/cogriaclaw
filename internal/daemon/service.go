package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	launchdLabel = "com.cogria.cogriaclaw"
	systemdUnit  = "cogriaclaw.service"
)

// ServiceParams describes how to launch cogriaclaw as a managed service.
type ServiceParams struct {
	BinPath    string // absolute path to the cogriaclaw binary
	ConfigPath string // absolute path to config.yaml
	WorkDir    string // working directory (where data/ and skills/ resolve)
}

// Install registers cogriaclaw as a user-level native service and starts it.
// macOS → LaunchAgent; Linux → systemd user unit. No root required; the
// trade-off is boot-start behaviour (see the printed hints).
func Install(configPath string) error {
	p, err := resolveParams(configPath)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(p)
	case "linux":
		return installSystemd(p)
	default:
		return fmt.Errorf("install: unsupported OS %q (macOS and Linux only)", runtime.GOOS)
	}
}

// ServiceInstalled reports whether a native service unit exists for cogriaclaw.
func ServiceInstalled() bool {
	var path string
	var err error
	switch runtime.GOOS {
	case "darwin":
		path, err = launchdPlistPath()
	case "linux":
		path, err = systemdUnitPath()
	default:
		return false
	}
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
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
		return fmt.Errorf("restart: unsupported OS %q", runtime.GOOS)
	}
}

// Uninstall stops and removes the service.
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return fmt.Errorf("uninstall: unsupported OS %q", runtime.GOOS)
	}
}

func resolveParams(configPath string) (ServiceParams, error) {
	bin, err := os.Executable()
	if err != nil {
		return ServiceParams{}, fmt.Errorf("locate binary: %w", err)
	}
	bin, _ = filepath.Abs(bin)
	cfg, err := filepath.Abs(configPath)
	if err != nil {
		return ServiceParams{}, fmt.Errorf("resolve config path: %w", err)
	}
	if _, err := os.Stat(cfg); err != nil {
		return ServiceParams{}, fmt.Errorf("config not found at %s — run install from where config.yaml lives, or pass -config", cfg)
	}
	return ServiceParams{BinPath: bin, ConfigPath: cfg, WorkDir: filepath.Dir(cfg)}, nil
}

// ---- macOS: LaunchAgent ----

func launchdPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func installLaunchd(p ServiceParams) error {
	plist, err := launchdPlistPath()
	if err != nil {
		return err
	}
	logPath := filepath.Join(p.WorkDir, "cogriaclaw.log")
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
`, launchdLabel, p.BinPath, p.ConfigPath, p.WorkDir, logPath, logPath)

	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	_ = exec.Command("launchctl", "unload", plist).Run() // ignore: may not be loaded
	if out, err := exec.Command("launchctl", "load", "-w", plist).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %v: %s", err, out)
	}
	fmt.Printf("Installed LaunchAgent: %s\n", plist)
	fmt.Printf("Logs: %s\n", logPath)
	fmt.Println("It starts at login and restarts on crash. Manage with: cogriaclaw {reload,stop,status} or launchctl.")
	return nil
}

func uninstallLaunchd() error {
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
}

// ---- Linux: systemd user unit ----

func systemdUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdUnit), nil
}

func installSystemd(p ServiceParams) error {
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
		return fmt.Errorf("write unit: %w", err)
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "--now", systemdUnit},
	} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	fmt.Printf("Installed systemd user unit: %s\n", unitPath)
	fmt.Println("Manage with: cogriaclaw {reload,stop,status} or systemctl --user {status,reload,stop} cogriaclaw")
	if u, err := user.Current(); err == nil {
		fmt.Printf("To keep it running after logout / across reboot:\n  sudo loginctl enable-linger %s\n", u.Username)
	}
	return nil
}

func uninstallSystemd() error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", systemdUnit).Run()
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit: %w", err)
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Printf("Removed systemd user unit: %s\n", unitPath)
	return nil
}
