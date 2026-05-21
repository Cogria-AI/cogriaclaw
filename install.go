package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Cogria-AI/cogriaclaw/internal/daemon"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

const appDirName = ".cogriaclaw"

// installService copies the binary to ~/.local/bin and the config/skills/session
// to ~/.cogriaclaw, registers a native service pointing at those internal-disk
// paths (a LaunchAgent can't exec from an external volume), and either starts it
// (if already logged in) or prints the login-first instructions.
func installService(srcConfigPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	srcAbs, err := filepath.Abs(srcConfigPath)
	if err != nil {
		return err
	}
	if !exists(srcAbs) {
		return fmt.Errorf("config not found at %s — run install from your project directory, or pass -config", srcAbs)
	}
	srcDir := filepath.Dir(srcAbs)

	binSrc, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}

	binDir := filepath.Join(home, ".local", "bin")
	binDst := filepath.Join(binDir, "cogriaclaw")
	appDir := filepath.Join(home, appDirName)
	cfgDst := filepath.Join(appDir, "config.yaml")
	logPath := filepath.Join(appDir, "cogriaclaw.log")
	dataDir := filepath.Join(appDir, "data")

	// 1. binary → ~/.local/bin (internal disk, on PATH for most setups)
	if err := copyFile(binSrc, binDst, 0o755); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}

	// 2. ~/.cogriaclaw: config (don't clobber an edited one), skills, session
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	if !exists(cfgDst) {
		if err := copyFile(srcAbs, cfgDst, 0o600); err != nil {
			return fmt.Errorf("copy config: %w", err)
		}
	}
	if src := filepath.Join(srcDir, "skills"); exists(src) {
		if err := copyTree(src, filepath.Join(appDir, "skills")); err != nil {
			return fmt.Errorf("copy skills: %w", err)
		}
	}
	// Carry over an existing logged-in session so the service is ready to run.
	if src := filepath.Join(srcDir, "data"); exists(src) && !exists(dataDir) {
		if err := copyTree(src, dataDir); err != nil {
			return fmt.Errorf("copy session data: %w", err)
		}
	}

	// 3. register the service at the installed paths (does not start it)
	if err := daemon.RegisterService(daemon.ServiceParams{
		BinPath:    binDst,
		ConfigPath: cfgDst,
		WorkDir:    appDir,
		LogPath:    logPath,
	}); err != nil {
		return err
	}

	fmt.Println("Installed:")
	fmt.Printf("  binary  %s\n", binDst)
	fmt.Printf("  config  %s\n", cfgDst)
	fmt.Printf("  data    %s\n", dataDir)

	if !onPath(binDir) {
		fmt.Printf("\nNote: %s is not on your PATH. Add it so you can call 'cogriaclaw' directly:\n", binDir)
		fmt.Println(`  echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc`)
	}

	// 4. login-aware next step
	if wa.HasSession(dataDir) {
		if err := daemon.StartService(); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
		fmt.Println("\nAlready logged in — service started. It restarts on crash and starts at login.")
	} else {
		fmt.Println("\nNot logged in yet. Log in once, then start the background service:")
		fmt.Println("  cogriaclaw run      # scan the QR (WhatsApp -> Linked Devices), then Ctrl+C")
		fmt.Println("  cogriaclaw start    # run as a background service")
	}
	if hint := daemon.LingerHint(); hint != "" {
		fmt.Printf("\nTo keep running after logout/reboot:\n  %s\n", hint)
	}

	fmt.Println("\nCommands: cogriaclaw status | reload | stop | start | restart | uninstall | help")
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func onPath(dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == dir {
			return true
		}
	}
	return false
}
