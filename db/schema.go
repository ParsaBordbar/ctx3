package db

import (
	"sort"
	"strings"
)

// schemaBuilder accumulates tables across every source, keyed by lowercase table name so a table created in a migration and later altered in another file lands in one place.
type schemaBuilder struct {
	tables  map[string]*Table
	order   []string // insertion order of keys; final output is name-sorted
	sources []string
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{tables: map[string]*Table{}}
}

func key(name string) string { return strings.ToLower(name) }

// table returns the accumulator for name, creating it on first sight.
func (b *schemaBuilder) table(name, source string) *Table {
	k := key(name)
	t, ok := b.tables[k]
	if !ok {
		t = &Table{Name: name, Source: source}
		b.tables[k] = t
		b.order = append(b.order, k)
	}
	return t
}

// addColumn appends col to a table, ignoring a duplicate column name (the first definition wins, later files only add).
func (b *schemaBuilder) addColumn(tableName, source string, col Column) {
	t := b.table(tableName, source)
	for _, c := range t.Columns {
		if key(c.Name) == key(col.Name) {
			return
		}
	}
	t.Columns = append(t.Columns, col)
}

// markPK/markUnique/markRef apply constraints declared apart from the column (table-level PRIMARY KEY (...), ALTER TABLE ... ADD FOREIGN KEY, CREATE UNIQUE INDEX).
func (b *schemaBuilder) mark(tableName, colName string, apply func(*Column)) {
	t, ok := b.tables[key(tableName)]
	if !ok {
		return
	}
	for i := range t.Columns {
		if key(t.Columns[i].Name) == key(colName) {
			apply(&t.Columns[i])
			return
		}
	}
}

func (b *schemaBuilder) markPK(table, col string) {
	b.mark(table, col, func(c *Column) { c.PrimaryKey = true; c.NotNull = true })
}

func (b *schemaBuilder) markUnique(table, col string) {
	b.mark(table, col, func(c *Column) { c.Unique = true })
}

func (b *schemaBuilder) markRef(table, col, refTable, refCol string) {
	if refCol == "" {
		refCol = "id"
	}
	b.mark(table, col, func(c *Column) { c.Ref = &Ref{Table: refTable, Column: refCol} })
}

// drop removes a table (DROP TABLE in a later migration) so the folded schema reflects the end state, not every table that ever existed.
func (b *schemaBuilder) drop(name string) {
	k := key(name)
	if _, ok := b.tables[k]; !ok {
		return
	}
	delete(b.tables, k)
	for i, o := range b.order {
		if o == k {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
}

func (b *schemaBuilder) dropColumn(table, col string) {
	t, ok := b.tables[key(table)]
	if !ok {
		return
	}
	for i, c := range t.Columns {
		if key(c.Name) == key(col) {
			t.Columns = append(t.Columns[:i], t.Columns[i+1:]...)
			return
		}
	}
}

func (b *schemaBuilder) addSource(rel string) {
	b.sources = append(b.sources, rel)
}

// build finalizes the schema: tables sorted by name, relations derived from FK columns, cardinality inferred from whether the FK column is itself unique.
func (b *schemaBuilder) build() *Schema {
	s := &Schema{Sources: sortedUnique(b.sources)}
	for _, k := range b.order {
		s.Tables = append(s.Tables, *b.tables[k])
	}
	sort.Slice(s.Tables, func(i, j int) bool { return s.Tables[i].Name < s.Tables[j].Name })

	for _, t := range s.Tables {
		for _, c := range t.Columns {
			if c.Ref == nil {
				continue
			}
			card := "many-to-one"
			if c.Unique || c.PrimaryKey {
				card = "one-to-one"
			}
			s.Relations = append(s.Relations, Relation{
				FromTable: t.Name, FromColumn: c.Name,
				ToTable: c.Ref.Table, ToColumn: c.Ref.Column,
				Cardinality: card,
			})
		}
	}
	sort.Slice(s.Relations, func(i, j int) bool {
		a, b := s.Relations[i], s.Relations[j]
		if a.FromTable != b.FromTable {
			return a.FromTable < b.FromTable
		}
		return a.FromColumn < b.FromColumn
	})
	return s
}

// Table returns the named table (case-insensitive), or nil.
func (s *Schema) Table(name string) *Table {
	for i := range s.Tables {
		if key(s.Tables[i].Name) == key(name) {
			return &s.Tables[i]
		}
	}
	return nil
}

// Referrers returns the relations pointing at the named table.
func (s *Schema) Referrers(name string) []Relation {
	var out []Relation
	for _, r := range s.Relations {
		if key(r.ToTable) == key(name) {
			out = append(out, r)
		}
	}
	return out
}
