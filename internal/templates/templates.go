// Package templates provides the embedded LaTeX/Markdown/Lua templates
// used by the report pipeline.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/tex/* templates/md/* templates/lua/*
var FS embed.FS

// Install extracts all embedded templates to ~/.local/share/delve-debug/.
func Install() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	dest := filepath.Join(home, ".local", "share", "delve-debug")
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dest, err)
	}
	for _, sub := range []string{"templates/tex", "templates/md", "templates/lua"} {
		if err := InstallDir(sub, dest); err != nil {
			return err
		}
	}
	return nil
}

// InstallDir copies all files from the embedded sub-directory dir into dest.
func InstallDir(dir string, dest string) error {
	return fs.WalkDir(FS, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		out := filepath.Join(dest, filepath.Base(path))
		if err := os.WriteFile(out, content, 0644); err != nil {
			return err
		}
		fmt.Printf("installed %s\n", out)
		return nil
	})
}
