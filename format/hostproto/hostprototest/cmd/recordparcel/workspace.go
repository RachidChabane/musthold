package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	editedFile      = "parcel/rates.py"
	editText        = "\n# reviewed\n"
	baseCommitTitle = "base"
	baseBranch      = "main"
	versionControl  = "git"
	commitDate      = "2026-01-01T00:00:00+00:00"
	directoryMode   = 0o755
	fileMode        = 0o644
)

// commitIdentity is the environment of every command that builds the tree: one
// author, one date and no user or system configuration, so the base commit is the
// same on every machine.
var commitIdentity = []string{
	"GIT_AUTHOR_NAME=bench",
	"GIT_AUTHOR_EMAIL=bench@example.invalid",
	"GIT_COMMITTER_NAME=bench",
	"GIT_COMMITTER_EMAIL=bench@example.invalid",
	"GIT_AUTHOR_DATE=" + commitDate,
	"GIT_COMMITTER_DATE=" + commitDate,
	"GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_CONFIG_SYSTEM=" + os.DevNull,
}

// BuildParcelTree copies the parcel fixture to destination, commits it, and leaves
// one uncommitted edit: what a coder session leaves behind.
func BuildParcelTree(fixture, destination string) error {
	if err := copyDirectory(fixture, destination); err != nil {
		return err
	}
	for _, arguments := range [][]string{
		{"init", "--initial-branch", baseBranch},
		{"add", "--all"},
		{"commit", "--message", baseCommitTitle},
	} {
		if err := runVersionControl(destination, arguments...); err != nil {
			return err
		}
	}
	edited := filepath.Join(destination, editedFile)
	content, err := os.ReadFile(edited)
	if err != nil {
		return err
	}
	return os.WriteFile(edited, append(content, editText...), fileMode)
}

func runVersionControl(directory string, arguments ...string) error {
	command := exec.Command(versionControl, append([]string{"-C", directory}, arguments...)...)
	command.Env = append(os.Environ(), commitIdentity...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", versionControl, arguments, err, output)
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, directoryMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}
