// Embedded LaTeX/Markdown templates and install command.
package delvehelper

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/tex/* templates/md/* templates/lua/*
var templateFS embed.FS

func cmdInstallTemplates() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	dest := filepath.Join(home, ".local", "share", "delve-debug")
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dest, err)
	}
	if err := installTemplatesDir("templates/tex", dest); err != nil {
		return err
	}
	if err := installTemplatesDir("templates/md", dest); err != nil {
		return err
	}
	if err := installTemplatesDir("templates/lua", dest); err != nil {
		return err
	}
	return nil
}

func installTemplatesDir(dir string, dest string) error {
	return fs.WalkDir(templateFS, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := templateFS.ReadFile(path)
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
