// Package integrations embeds the agent skills, commands and instruction
// blocks that `baton integrate` installs. A new integration is a directory
// here plus an entry in Registry.
package integrations

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

//go:embed claude opencode
var files embed.FS

// Marker identifies files written by baton; others are never overwritten
// without --force or removed.
const Marker = "managed by baton integrate"

const (
	blockBegin = "<!-- baton:begin (managed by baton integrate) -->"
	blockEnd   = "<!-- baton:end -->"
)

// File maps an embedded file to a path under the agent's config directory.
type File struct{ Src, Dst string }

// Integration is one agent.
type Integration struct {
	Name  string
	Base  func(home string) string // agent config directory
	Files []File
	Block File // instruction block appended to an existing instructions file
}

// Registry lists the supported agents.
var Registry = []Integration{
	{
		Name: "claude",
		Base: func(home string) string {
			if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
				return d
			}
			return filepath.Join(home, ".claude")
		},
		Files: []File{
			{"claude/skills/handoff/SKILL.md", "skills/handoff/SKILL.md"},
			{"claude/skills/pickup/SKILL.md", "skills/pickup/SKILL.md"},
			{"claude/skills/tips/SKILL.md", "skills/tips/SKILL.md"},
		},
		Block: File{"claude/CLAUDE.md", "CLAUDE.md"},
	},
	{
		Name: "opencode",
		Base: func(home string) string {
			if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
				return filepath.Join(d, "opencode")
			}
			return filepath.Join(home, ".config", "opencode")
		},
		Files: []File{
			{"opencode/commands/handoff.md", "commands/handoff.md"},
			{"opencode/commands/pickup.md", "commands/pickup.md"},
			{"opencode/commands/tips.md", "commands/tips.md"},
		},
		Block: File{"opencode/AGENTS.md", "AGENTS.md"},
	},
}

// Names returns the registered agent names.
func Names() []string {
	out := make([]string, len(Registry))
	for i, in := range Registry {
		out[i] = in.Name
	}
	return out
}

// Get finds an integration by name.
func Get(name string) (Integration, error) {
	for _, in := range Registry {
		if in.Name == name {
			return in, nil
		}
	}
	return Integration{}, fmt.Errorf("unknown integration %q (have: %s)", name, strings.Join(Names(), ", "))
}

// Read returns an embedded file.
func Read(src string) ([]byte, error) { return files.ReadFile(src) }

// Installed reports whether any managed file of the integration exists.
func (in Integration) Installed(home string) bool {
	base := in.Base(home)
	for _, f := range in.Files {
		if data, err := os.ReadFile(filepath.Join(base, f.Dst)); err == nil && strings.Contains(string(data), Marker) {
			return true
		}
	}
	return false
}

// Install writes the files and the instruction block. Existing files not
// written by baton are left alone unless force is set. It returns the
// paths written.
func (in Integration) Install(home string, force bool) ([]string, error) {
	base := in.Base(home)
	var written []string
	var errs []error
	for _, f := range in.Files {
		data, err := files.ReadFile(f.Src)
		if err != nil {
			return written, err
		}
		dst := filepath.Join(base, f.Dst)
		if cur, err := os.ReadFile(dst); err == nil && !strings.Contains(string(cur), Marker) && !force {
			errs = append(errs, fmt.Errorf("%s exists and was not written by baton; move it away or pass --force", dst))
			continue
		}
		if err := fsutil.WriteFileAtomic(dst, data, 0o644); err != nil {
			return written, err
		}
		written = append(written, dst)
	}
	block, err := files.ReadFile(in.Block.Src)
	if err != nil {
		return written, err
	}
	dst := filepath.Join(base, in.Block.Dst)
	if err := writeBlock(dst, string(block)); err != nil {
		return written, err
	}
	written = append(written, dst)
	return written, errors.Join(errs...)
}

// Uninstall removes managed files and the instruction block.
func (in Integration) Uninstall(home string) ([]string, error) {
	base := in.Base(home)
	var removed []string
	for _, f := range in.Files {
		dst := filepath.Join(base, f.Dst)
		data, err := os.ReadFile(dst)
		if err != nil || !strings.Contains(string(data), Marker) {
			continue
		}
		if err := os.Remove(dst); err != nil {
			return removed, err
		}
		removed = append(removed, dst)
		// Drop now-empty skill directories.
		if dir := filepath.Dir(dst); dir != base {
			os.Remove(dir)
		}
	}
	dst := filepath.Join(base, in.Block.Dst)
	ok, err := removeBlock(dst)
	if err != nil {
		return removed, err
	}
	if ok {
		removed = append(removed, dst+" (baton block)")
	}
	return removed, nil
}

func writeBlock(path, content string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	block := blockBegin + "\n" + strings.TrimRight(content, "\n") + "\n" + blockEnd + "\n"
	if i := strings.Index(text, blockBegin); i >= 0 {
		if j := strings.Index(text[i:], blockEnd); j >= 0 {
			end := i + j + len(blockEnd)
			if end < len(text) && text[end] == '\n' {
				end++
			}
			text = text[:i] + block + text[end:]
			return fsutil.WriteFileAtomic(path, []byte(text), 0o644)
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return fsutil.WriteFileAtomic(path, []byte(text+block), 0o644)
}

func removeBlock(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	i := strings.Index(text, blockBegin)
	if i < 0 {
		return false, nil
	}
	j := strings.Index(text[i:], blockEnd)
	if j < 0 {
		return false, fmt.Errorf("%s: unterminated baton block", path)
	}
	end := i + j + len(blockEnd)
	if end < len(text) && text[end] == '\n' {
		end++
	}
	before := strings.TrimRight(text[:i], "\n")
	after := strings.TrimLeft(text[end:], "\n")
	out := before
	if before != "" && after != "" {
		out += "\n\n" + after
	} else if after != "" {
		out = after
	} else if before != "" {
		out += "\n"
	}
	if strings.TrimSpace(out) == "" {
		return true, os.Remove(path)
	}
	return true, fsutil.WriteFileAtomic(path, []byte(out), 0o644)
}
