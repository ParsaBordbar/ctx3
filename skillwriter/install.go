package skillwriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Install copies a hand-written skill into cfg.SkillsDir under its own name.
// The whole source tree comes along (SKILL.md, reference/, scripts, assets) so
// a skill that ships helper files keeps working after the move.
//
// Name overrides the frontmatter name when non-empty — useful to disambiguate a
// generic "review" skill into "backend-review" at install time.
func Install(cfg Config, src Source, name string) (dir string, files []string, err error) {
	if cfg.SkillsDir == "" {
		return "", nil, fmt.Errorf("SkillsDir is empty — the selected target does not support skills")
	}
	if name == "" {
		name = src.Meta.Name
	}
	if !isKebab(name) {
		return "", nil, fmt.Errorf("skill name %q must be kebab-case (a-z, 0-9, -)", name)
	}

	dir = filepath.Join(cfg.SkillsDir, name)
	if abs, absErr := filepath.Abs(dir); absErr == nil {
		if srcAbs, srcErr := filepath.Abs(src.Dir); srcErr == nil && abs == srcAbs {
			return "", nil, fmt.Errorf("%s is already the installed skill — nothing to copy", dir)
		}
	}
	if _, statErr := os.Stat(dir); statErr == nil && !cfg.Force {
		return "", nil, fmt.Errorf("%s already exists (use --force to overwrite)", dir)
	}

	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}

	for _, rel := range src.Files {
		from := filepath.Join(src.Dir, rel)
		to := filepath.Join(dir, rel)
		if err = os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return "", nil, err
		}
		if err = copyFile(from, to); err != nil {
			return "", nil, err
		}
		files = append(files, to)
	}

	// The installed copy is the one the agent reads: if --name renamed it, the
	// frontmatter must agree or the agent indexes it under the old name.
	if name != src.Meta.Name {
		if err = rewriteName(filepath.Join(dir, "SKILL.md"), name); err != nil {
			return "", nil, err
		}
	}
	return dir, files, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// rewriteName replaces the frontmatter `name:` of an installed SKILL.md.
func rewriteName(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "name:") {
			lines[i] = "name: " + name
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
		}
		if i > 0 && strings.TrimSpace(line) == "---" {
			break // end of frontmatter, no name key
		}
	}
	// No name key in frontmatter — insert one right after the opening fence.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		lines = append(lines[:1], append([]string{"name: " + name}, lines[1:]...)...)
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return fmt.Errorf("%s: cannot set name — no frontmatter block", path)
}

// Entry is one installed skill found on disk.
type Entry struct {
	Name        string
	Dir         string
	Description string
	Err         error // non-nil when SKILL.md is missing or malformed
}

// List returns the skills installed directly under dir, sorted by name. A
// missing dir is not an error — an unused scope is simply empty.
func List(dir string) ([]Entry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Entry
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		entry := Entry{Name: e.Name(), Dir: sub}
		data, readErr := os.ReadFile(filepath.Join(sub, "SKILL.md"))
		if readErr != nil {
			entry.Err = fmt.Errorf("no SKILL.md")
			out = append(out, entry)
			continue
		}
		meta, parseErr := ParseMeta(string(data))
		if parseErr != nil {
			entry.Err = parseErr
		} else {
			entry.Description = meta.Description
			if meta.Name != "" {
				entry.Name = meta.Name
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// pluginScanDepth bounds the walk under the plugins root. Plugin skills sit at
// <root>/<marketplace>/<plugin>/skills/<name>/SKILL.md, sometimes one level
// deeper, so a shallow cap keeps a large cache from being walked end to end.
const pluginScanDepth = 6

// pluginName derives the owning plugin from a .../<plugin>/skills path. The
// cache interposes a version directory (<plugin>/<hash>/skills, or the literal
// "unknown"), so those are stepped over.
func pluginName(skillsPath string) string {
	dir := filepath.Dir(skillsPath)
	for range 2 {
		base := filepath.Base(dir)
		if base != "unknown" && !isVersionHash(base) {
			return base
		}
		dir = filepath.Dir(dir)
	}
	return filepath.Base(dir)
}

func isVersionHash(s string) bool {
	if len(s) < 6 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// ListPlugins finds skills shipped by installed plugins under root. Each entry
// is named the way an agent refers to it — `plugin:skill` — because that
// namespacing is exactly why a plugin skill never collides with a personal one.
func ListPlugins(root string) ([]Entry, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var out []Entry
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // unreadable corner of the cache — keep going
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if depth := len(strings.Split(filepath.ToSlash(rel), "/")); rel != "." && depth > pluginScanDepth {
			return filepath.SkipDir
		}
		name := d.Name()
		if name == ".git" || name == "node_modules" {
			return filepath.SkipDir
		}
		if name != "skills" {
			return nil
		}
		plugin := pluginName(p)
		entries, listErr := List(p)
		if listErr != nil {
			return nil
		}
		for _, e := range entries {
			if e.Err != nil {
				continue // not every dir under skills/ is a skill
			}
			e.Name = plugin + ":" + e.Name
			out = append(out, e)
		}
		return filepath.SkipDir
	})
	if err != nil {
		return out, err
	}

	// The same plugin is cached per version and mirrored under marketplaces/,
	// so the same skill shows up several times. Report each name once.
	seen := make(map[string]bool, len(out))
	deduped := out[:0]
	for _, e := range out {
		if seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		deduped = append(deduped, e)
	}
	out = deduped
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
