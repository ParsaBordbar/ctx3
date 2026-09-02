// Package gitfacts reports what a repository's history says about the code:
// which files churn, what changed recently, and what is uncommitted right now.
//
// Every other ctx3 package describes the code as it stands. This one describes
// how it got there — the one question an agent cannot answer by reading files,
// and usually the fastest route to "what is being worked on here".
//
// It shells out to git rather than linking a git library: the binary is already
// present wherever a checkout is, and the plumbing commands used here have been
// stable for years.
package gitfacts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config controls a scan.
type Config struct {
	// RootDir is the repository (or any directory inside it).
	RootDir string
	// Commits is how many recent commits to report (default 15).
	Commits int
	// ChurnWindow is how many commits back to measure churn over (default 200).
	ChurnWindow int
	// TopFiles is how many hot files to list (default 15).
	TopFiles int
}

// Commit is one revision.
type Commit struct {
	SHA     string `json:"sha"     toon:"sha"`
	Author  string `json:"author"  toon:"author"`
	Date    string `json:"date"    toon:"date"`
	Subject string `json:"subject" toon:"subject"`
}

// FileChurn is how often one path changed in the window.
type FileChurn struct {
	Path    string `json:"path"    toon:"path"`
	Commits int    `json:"commits" toon:"commits"`
	// LastChanged is the ISO date of the most recent commit touching the path.
	LastChanged string `json:"lastChanged" toon:"last_changed"`
}

// Change is one uncommitted path and its porcelain status code.
type Change struct {
	Path   string `json:"path"   toon:"path"`
	Status string `json:"status" toon:"status"`
}

// Report is the completed scan.
type Report struct {
	Root        string      `json:"root"      toon:"root"`
	Branch      string      `json:"branch"    toon:"branch"`
	Head        string      `json:"head"      toon:"head"`
	HeadDate    string      `json:"headDate"  toon:"head_date"`
	Remote      string      `json:"remote,omitempty" toon:"remote,omitempty"`
	Commits     []Commit    `json:"commits"   toon:"commits"`
	Uncommitted []Change    `json:"uncommitted" toon:"uncommitted"`
	Churn       []FileChurn `json:"churn"     toon:"churn"`
	// ChurnWindow is how many commits the churn counts were measured over,
	// so a count can be read as a rate rather than an absolute.
	ChurnWindow int `json:"churnWindow" toon:"churn_window"`
	// Authors is the distinct commit authors seen in the churn window.
	Authors int `json:"authors" toon:"authors"`
}

// ErrNotARepo is returned when RootDir is not inside a git checkout.
var ErrNotARepo = errors.New("not a git repository")

// unitSep separates fields in git's output. It cannot appear in a commit
// subject, unlike any printable delimiter.
const unitSep = "\x00"

// unitSepFmt is how that separator is *requested*: git expands "%x00" to a NUL
// in its output. The byte itself cannot be passed here — an argv element is a
// NUL-terminated C string, so embedding one makes exec reject the command, and
// the log comes back empty.
const unitSepFmt = "%x00"

// Analyze collects the repository facts under cfg.RootDir.
func Analyze(cfg Config) (*Report, error) {
	if cfg.Commits <= 0 {
		cfg.Commits = 15
	}
	if cfg.ChurnWindow <= 0 {
		cfg.ChurnWindow = 200
	}
	if cfg.TopFiles <= 0 {
		cfg.TopFiles = 15
	}
	root := cfg.RootDir
	if root == "" {
		root = "."
	}

	g := &runner{dir: root}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH: %w", err)
	}
	if out, err := g.run("rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return nil, ErrNotARepo
	}

	rep := &Report{Root: root, ChurnWindow: cfg.ChurnWindow}

	// A repository with no commits yet answers rev-parse HEAD with an error;
	// that is an empty history, not a failure.
	if head, err := g.run("rev-parse", "--short", "HEAD"); err == nil {
		rep.Head = strings.TrimSpace(head)
	}
	if branch, err := g.run("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		rep.Branch = strings.TrimSpace(branch)
	}
	if remote, err := g.run("remote", "get-url", "origin"); err == nil {
		rep.Remote = strings.TrimSpace(remote)
	}

	rep.Commits = g.commits(cfg.Commits)
	if len(rep.Commits) > 0 {
		rep.HeadDate = rep.Commits[0].Date
	}
	rep.Uncommitted = g.status()
	rep.Churn, rep.Authors = g.churn(cfg.ChurnWindow, cfg.TopFiles)

	return rep, nil
}

// runner executes git in a fixed directory.
type runner struct{ dir string }

func Run(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH: %w", err)
	}
	return (&runner{dir: dir}).run(args...)
}

func IsRepo(dir string) bool {
	out, err := Run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func (r *runner) run(args ...string) (string, error) {
	// A hung git (a credential prompt, a slow filesystem) must not hang the
	// caller, which may be an MCP server answering a request.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	// Never prompt: in a non-interactive context a prompt is an indefinite hang.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// commits reads the most recent n revisions.
func (r *runner) commits(n int) []Commit {
	format := strings.Join([]string{"%h", "%an", "%aI", "%s"}, unitSepFmt)
	out, err := r.run("log", "-n", strconv.Itoa(n), "--no-merges", "--pretty=format:"+format)
	if err != nil {
		return nil
	}

	var commits []Commit
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, unitSep)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{
			SHA: parts[0], Author: parts[1], Date: dateOnly(parts[2]), Subject: parts[3],
		})
	}
	return commits
}

// status lists uncommitted paths. This is the working set — what someone has
// their hands in right now — and is the single most useful fact here when an
// agent joins a session already in progress.
func (r *runner) status() []Change {
	out, err := r.run("status", "--porcelain")
	if err != nil {
		return nil
	}

	var changes []Change
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		// A rename reads "old -> new"; the new path is the one that matters.
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		changes = append(changes, Change{Path: strings.Trim(path, `"`), Status: statusWord(code)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// churn counts how many of the last window commits touched each path, and how
// many distinct authors appear. A file near the top is where the work is.
func (r *runner) churn(window, top int) ([]FileChurn, int) {
	format := "C" + unitSepFmt + "%an" + unitSepFmt + "%aI"
	out, err := r.run("log", "-n", strconv.Itoa(window), "--no-merges",
		"--name-only", "--pretty=format:"+format)
	if err != nil {
		return nil, 0
	}

	counts := map[string]int{}
	last := map[string]string{}
	authors := map[string]bool{}
	date := ""

	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "C"+unitSep); ok {
			parts := strings.Split(rest, unitSep)
			if len(parts) == 2 {
				authors[parts[0]] = true
				date = dateOnly(parts[1])
			}
			continue
		}
		counts[line]++
		// Commits arrive newest first, so the first date seen is the latest.
		if _, seen := last[line]; !seen {
			last[line] = date
		}
	}

	churn := make([]FileChurn, 0, len(counts))
	for path, n := range counts {
		churn = append(churn, FileChurn{Path: path, Commits: n, LastChanged: last[path]})
	}
	sort.Slice(churn, func(i, j int) bool {
		if churn[i].Commits != churn[j].Commits {
			return churn[i].Commits > churn[j].Commits
		}
		return churn[i].Path < churn[j].Path
	})
	if len(churn) > top {
		churn = churn[:top]
	}
	return churn, len(authors)
}

// statusWord expands a porcelain code into a word, since "??" and "M" mean
// nothing to a reader who has not memorized git's table.
func statusWord(code string) string {
	switch code {
	case "??":
		return "untracked"
	case "M", "MM", "AM":
		return "modified"
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "U", "UU", "AA", "DD":
		return "conflicted"
	}
	if strings.HasPrefix(code, "R") {
		return "renamed"
	}
	if strings.HasPrefix(code, "D") {
		return "deleted"
	}
	return "modified"
}

// dateOnly trims an ISO-8601 timestamp to its date. The clock time is noise
// for every question this package answers.
func dateOnly(iso string) string {
	if i := strings.IndexByte(iso, 'T'); i > 0 {
		return iso[:i]
	}
	return strings.TrimSpace(iso)
}
