// Package db discovers which databases a project uses and.
package db

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind classifies a detected engine by data model.
type Kind string

const (
	KindRelational Kind = "relational"
	KindDocument   Kind = "document"
	KindKeyValue   Kind = "key-value"
	KindSearch     Kind = "search"
	KindGraph      Kind = "graph"
	KindColumnar   Kind = "columnar"
	KindVector     Kind = "vector"
	KindEmbedded   Kind = "embedded"
)

// Engine is one database detected in the project, with the evidence that proved it.
type Engine struct {
	Name     string   `json:"name" toon:"name"`         // "postgres", "redis", ...
	Kind     Kind     `json:"kind" toon:"kind"`         // data model
	Evidence []string `json:"evidence" toon:"evidence"` // "go.mod: github.com/lib/pq"
}

// Ref is a foreign-key target.
type Ref struct {
	Table  string `json:"table" toon:"table"`
	Column string `json:"column" toon:"column"`
}

// Column is one column of a relational table.
type Column struct {
	Name       string `json:"name" toon:"name"`
	Type       string `json:"type" toon:"type"`
	PrimaryKey bool   `json:"primary_key" toon:"primary_key"`
	Unique     bool   `json:"unique" toon:"unique"`
	NotNull    bool   `json:"not_null" toon:"not_null"`
	Ref        *Ref   `json:"ref,omitempty" toon:"ref,omitempty"` // set when this column is a FK
}

// Table is one relational table.
type Table struct {
	Name    string   `json:"name" toon:"name"`
	Columns []Column `json:"columns" toon:"columns"`
	Source  string   `json:"source" toon:"source"` // repo-relative file it was parsed from
}

// Relation is a foreign-key edge between two tables.
type Relation struct {
	FromTable   string `json:"from_table" toon:"from_table"`
	FromColumn  string `json:"from_column" toon:"from_column"`
	ToTable     string `json:"to_table" toon:"to_table"`
	ToColumn    string `json:"to_column" toon:"to_column"`
	Cardinality string `json:"cardinality" toon:"cardinality"` // "many-to-one" | "one-to-one"
}

// Schema is the reconstructed relational schema, sorted by table name.
type Schema struct {
	Tables    []Table    `json:"tables" toon:"tables"`
	Relations []Relation `json:"relations" toon:"relations"`
	Sources   []string   `json:"sources" toon:"sources"` // files the schema was built from
}

// Report is the whole answer: what databases the project uses.
type Report struct {
	Project string   `json:"project" toon:"project"`
	Engines []Engine `json:"engines" toon:"engines"`
	Schema  *Schema  `json:"schema,omitempty" toon:"schema,omitempty"`
}

// Relational reports whether any detected engine stores relational data.
func (r *Report) Relational() bool {
	for _, e := range r.Engines {
		if e.Kind == KindRelational {
			return true
		}
	}
	return false
}

// Config controls analysis.
type Config struct {
	RootDir string
}

// Analyze detects the project's databases and reconstructs any relational schema it can find.
func Analyze(cfg Config) (*Report, error) {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}
	root, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, err
	}

	rep := &Report{
		Project: filepath.Base(root),
		Engines: Detect(root),
	}

	schema := parseSchema(root)
	if len(schema.Tables) > 0 {
		rep.Schema = schema
		// A schema proves a relational store even when no driver was detected (e.g. migrations committed before the app code).
		if !rep.Relational() {
			rep.Engines = append(rep.Engines, Engine{
				Name:     "sql",
				Kind:     KindRelational,
				Evidence: []string{"schema files: " + strings.Join(schema.Sources, ", ")},
			})
			sortEngines(rep.Engines)
		}
	}
	return rep, nil
}

// parseSchema merges every static schema source into one Schema.
func parseSchema(root string) *Schema {
	b := newSchemaBuilder()
	parseSQLFiles(root, b)
	parsePrisma(root, b)
	parseGoStructs(root, b)
	return b.build()
}

// walkFiles visits repo files in deterministic order, skipping the usual noise directories so a vendored dependency's migrations never leak into the schema.
func walkFiles(root string, fn func(path, rel string)) {
	var paths []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if path != root && (strings.HasPrefix(base, ".") && base != ".github" ||
				base == "vendor" || base == "node_modules" || base == "testdata" ||
				base == "dist" || base == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		fn(p, filepath.ToSlash(rel))
	}
}

func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
