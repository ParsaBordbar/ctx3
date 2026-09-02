package skillwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Meta is the frontmatter of a hand-written SKILL.md. Only name and description
// are load-bearing — the agent matches a task against description and loads the
// body only then, so an empty description makes the skill dead weight.
type Meta struct {
	Name        string
	Description string
	Extra       map[string]string // any other frontmatter keys, preserved verbatim
}

// ParseMeta reads the YAML-ish frontmatter block at the head of a SKILL.md.
// The format is deliberately narrow (flat `key: value` pairs between `---`
// fences) because that is all a SKILL.md frontmatter may contain.
func ParseMeta(src string) (Meta, error) {
	m := Meta{Extra: map[string]string{}}

	body := strings.TrimLeft(src, "\ufeff \t\r\n")
	if !strings.HasPrefix(body, "---") {
		return m, fmt.Errorf("missing frontmatter: a SKILL.md must start with a `---` block containing name and description")
	}
	rest := body[3:]
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")

	end := strings.Index(rest, "\n---")
	if end < 0 {
		return m, fmt.Errorf("unterminated frontmatter: no closing `---`")
	}

	var key string
	for raw := range strings.SplitSeq(rest[:end], "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Continuation of a folded value (indented, no new key).
		if key != "" && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			cont := strings.TrimSpace(line)
			switch key {
			case "name":
				m.Name += " " + cont
			case "description":
				m.Description += " " + cont
			default:
				m.Extra[key] += " " + cont
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return m, fmt.Errorf("malformed frontmatter line %q (want `key: value`)", line)
		}
		key = strings.ToLower(strings.TrimSpace(k))
		val := strings.Trim(strings.TrimSpace(v), `"'`)
		// YAML block scalars (`description: >`): the value is the indented
		// lines that follow, not the marker itself.
		if val == ">" || val == "|" || val == ">-" || val == "|-" || val == ">+" || val == "|+" {
			val = ""
		}
		switch key {
		case "name":
			m.Name = val
		case "description":
			m.Description = val
		default:
			m.Extra[key] = val
		}
	}

	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
	for k, v := range m.Extra {
		m.Extra[k] = strings.TrimSpace(v)
	}
	return m, nil
}

// Source is a skill authored by hand on disk, ready to be installed.
type Source struct {
	Dir   string   // directory holding SKILL.md
	Meta  Meta     // parsed frontmatter
	Files []string // paths relative to Dir, including SKILL.md
}

// Load reads a hand-written skill from path, which may be either the skill
// directory or the SKILL.md inside it, and validates the invariants that decide
// whether an agent can use it at all.
func Load(path string) (Source, error) {
	var src Source

	info, err := os.Stat(path)
	if err != nil {
		return src, err
	}

	dir := path
	if !info.IsDir() {
		if filepath.Base(path) != "SKILL.md" {
			return src, fmt.Errorf("%s is not a skill: expected a directory or a SKILL.md file", path)
		}
		dir = filepath.Dir(path)
	}

	mdPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return src, fmt.Errorf("no SKILL.md in %s: %w", dir, err)
	}

	meta, err := ParseMeta(string(data))
	if err != nil {
		return src, fmt.Errorf("%s: %w", mdPath, err)
	}
	if meta.Name == "" {
		meta.Name = filepath.Base(strings.TrimSuffix(dir, string(filepath.Separator)))
	}
	if !isKebab(meta.Name) {
		return src, fmt.Errorf("%s: skill name %q must be kebab-case (a-z, 0-9, -)", mdPath, meta.Name)
	}
	if meta.Description == "" {
		return src, fmt.Errorf("%s: `description` is empty — it is the trigger the agent matches a task against", mdPath)
	}

	files, err := collectFiles(dir)
	if err != nil {
		return src, err
	}

	src.Dir = dir
	src.Meta = meta
	src.Files = files
	return src, nil
}

// collectFiles lists every regular file under dir, relative to dir, skipping
// version-control noise so a skill authored inside a git repo copies cleanly.
func collectFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel != "." && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}
