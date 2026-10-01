// Package gitinfo reads branch/commit metadata and builds staleness reports.
// Everything degrades to "no git data" when git or the repository is missing.
package gitinfo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const timeout = 5 * time.Second

func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\r\n"), err
}

// Info is the git state recorded in a handoff.
type Info struct {
	Branch string // empty when detached
	Commit string // short hash; empty in a repository without commits
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// Read returns branch and commit for dir; ok is false outside a repository.
func Read(dir string) (Info, bool) {
	if !IsRepo(dir) {
		return Info{}, false
	}
	var i Info
	i.Branch, _ = git(dir, "symbolic-ref", "--short", "-q", "HEAD")
	i.Commit, _ = git(dir, "rev-parse", "--short", "HEAD")
	return i, true
}

// Staleness compares a handoff's branch/commit with the repository now.
type Staleness struct {
	Repo          bool     `json:"repo"`
	Branch        string   `json:"branch,omitempty"`
	Commit        string   `json:"commit,omitempty"`
	CurrentBranch string   `json:"current_branch,omitempty"`
	CurrentCommit string   `json:"current_commit,omitempty"`
	BranchChanged bool     `json:"branch_changed"`
	CommitMissing bool     `json:"commit_missing"`
	Diverged      bool     `json:"diverged"` // commit exists but is not an ancestor of HEAD (rebased?)
	NewCommits    int      `json:"new_commits"`
	Log           []string `json:"log,omitempty"` // newest first, at most logLimit
	ChangedFiles  int      `json:"changed_files"` // files differing between commit and work tree
	Dirty         int      `json:"dirty"`         // uncommitted changes in the work tree
}

const logLimit = 10

// Stale builds the report. With no recorded commit only dirtiness and branch
// are reported.
func Stale(dir, branch, commit string) Staleness {
	s := Staleness{Branch: branch, Commit: commit}
	cur, ok := Read(dir)
	if !ok {
		return s
	}
	s.Repo = true
	s.CurrentBranch, s.CurrentCommit = cur.Branch, cur.Commit
	s.BranchChanged = branch != "" && cur.Branch != branch
	if st, err := git(dir, "status", "--porcelain"); err == nil && st != "" {
		s.Dirty = len(strings.Split(st, "\n"))
	}
	if commit == "" || cur.Commit == "" {
		return s
	}
	if _, err := git(dir, "rev-parse", "--verify", "--quiet", commit+"^{commit}"); err != nil {
		s.CommitMissing = true
		return s
	}
	if _, err := git(dir, "merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		s.Diverged = true
	}
	if n, err := git(dir, "rev-list", "--count", commit+"..HEAD"); err == nil {
		s.NewCommits, _ = strconv.Atoi(n)
	}
	if s.NewCommits > 0 {
		if l, err := git(dir, "log", "--oneline", "--no-decorate", "-n", strconv.Itoa(logLimit), commit+"..HEAD"); err == nil && l != "" {
			s.Log = strings.Split(l, "\n")
		}
	}
	if d, err := git(dir, "diff", "--name-only", commit); err == nil && d != "" {
		s.ChangedFiles = len(strings.Split(d, "\n"))
	}
	return s
}

// Fresh reports that nothing changed since the handoff.
func (s Staleness) Fresh() bool {
	return !s.BranchChanged && !s.CommitMissing && !s.Diverged && s.NewCommits == 0 && s.ChangedFiles == 0 && s.Dirty == 0
}

// Lines renders the report for humans and agents.
func (s Staleness) Lines() []string {
	if !s.Repo {
		if s.Commit != "" {
			return []string{"not a git repository here; handoff was saved at " + s.Commit}
		}
		return nil
	}
	var out []string
	if s.BranchChanged {
		out = append(out, fmt.Sprintf("branch changed: handoff on %s, now on %s", s.Branch, orDetached(s.CurrentBranch)))
	}
	switch {
	case s.Commit == "":
	case s.CommitMissing:
		out = append(out, fmt.Sprintf("commit %s no longer exists (rebased, amended or not fetched)", s.Commit))
	case s.Diverged:
		out = append(out, fmt.Sprintf("commit %s is not in the history of HEAD (rebased?); %d commits on HEAD since the merge base", s.Commit, s.NewCommits))
	case s.NewCommits > 0:
		out = append(out, fmt.Sprintf("%d new commit(s) since %s:", s.NewCommits, s.Commit))
	}
	if s.NewCommits > 0 && !s.CommitMissing {
		for _, l := range s.Log {
			out = append(out, "  "+l)
		}
		if s.NewCommits > len(s.Log) {
			out = append(out, fmt.Sprintf("  ... and %d more", s.NewCommits-len(s.Log)))
		}
	}
	if s.ChangedFiles > 0 {
		out = append(out, fmt.Sprintf("%d file(s) differ from %s (git diff --stat %s)", s.ChangedFiles, s.Commit, s.Commit))
	}
	if s.Dirty > 0 {
		out = append(out, fmt.Sprintf("working tree dirty: %d path(s) (git status)", s.Dirty))
	}
	if len(out) == 0 {
		out = append(out, "up to date: no changes since "+nonEmpty(s.Commit, "the handoff"))
	}
	return out
}

func orDetached(b string) string { return nonEmpty(b, "detached HEAD") }

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
