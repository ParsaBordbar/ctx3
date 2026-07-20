package db

import (
	"os"
	"regexp"
	"strings"
)

// parseSQLFiles folds every .sql file in the repo into the builder.
func parseSQLFiles(root string, b *schemaBuilder) {
	walkFiles(root, func(path, rel string) {
		if !strings.HasSuffix(strings.ToLower(rel), ".sql") || isDownMigration(rel) {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		body := upSection(string(data))
		stmts := splitStatements(stripSQLComments(body))
		before := len(b.order)
		for _, stmt := range stmts {
			applyStatement(b, stmt, rel)
		}
		if len(b.order) > before || strings.Contains(strings.ToUpper(body), "ALTER TABLE") {
			b.addSource(rel)
		}
	})
}

// isDownMigration recognizes the rollback half of the common migration naming conventions (golang-migrate, dbmate, node-pg-migrate).
func isDownMigration(rel string) bool {
	low := strings.ToLower(rel)
	return strings.Contains(low, ".down.") || strings.HasSuffix(low, "_down.sql") ||
		strings.Contains(low, "/down/")
}

var downMarker = regexp.MustCompile(`(?im)^\s*--\s*\+(goose|migrate)\s+down\b`)
var upMarker = regexp.MustCompile(`(?im)^\s*--\s*\+(goose|migrate)\s+up\b`)

// upSection trims a single-file migration (goose, sql-migrate) down to its Up half.
func upSection(sql string) string {
	if loc := upMarker.FindStringIndex(sql); loc != nil {
		sql = sql[loc[1]:]
	}
	if loc := downMarker.FindStringIndex(sql); loc != nil {
		sql = sql[:loc[0]]
	}
	return sql
}

// stripSQLComments removes -- line comments and /* */ blocks while leaving string literals intact.
func stripSQLComments(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\'' || s[i] == '"' || s[i] == '`':
			q := s[i]
			out.WriteByte(s[i])
			i++
			for i < len(s) {
				out.WriteByte(s[i])
				if s[i] == q {
					i++
					break
				}
				i++
			}
		case strings.HasPrefix(s[i:], "--"):
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case strings.HasPrefix(s[i:], "/*"):
			if j := strings.Index(s[i+2:], "*/"); j >= 0 {
				i += 2 + j + 2
			} else {
				i = len(s)
			}
		default:
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// splitStatements splits on semicolons outside quotes and parentheses.
func splitStatements(sql string) []string {
	var stmts []string
	var cur strings.Builder
	depth := 0
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch c {
		case '\'', '"', '`':
			cur.WriteByte(c)
			for i++; i < len(sql); i++ {
				cur.WriteByte(sql[i])
				if sql[i] == c {
					break
				}
			}
		case '(':
			depth++
			cur.WriteByte(c)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case ';':
			if depth == 0 {
				stmts = append(stmts, cur.String())
				cur.Reset()
			} else {
				cur.WriteByte(c)
			}
		default:
			cur.WriteByte(c)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

var (
	reCreateTable = regexp.MustCompile(`(?is)^\s*create\s+(?:temp\w*\s+)?table\s+(?:if\s+not\s+exists\s+)?([^\s(]+)\s*\(`)
	reAlterTable  = regexp.MustCompile(`(?is)^\s*alter\s+table\s+(?:only\s+)?([^\s]+)\s+(.*)$`)
	reDropTable   = regexp.MustCompile(`(?is)^\s*drop\s+table\s+(?:if\s+exists\s+)?([^\s;]+)`)
	reUniqueIndex = regexp.MustCompile(`(?is)^\s*create\s+unique\s+index\s+(?:if\s+not\s+exists\s+)?\S+\s+on\s+([^\s(]+)\s*\(([^)]*)\)`)
	reReferences  = regexp.MustCompile(`(?is)references\s+([^\s(]+)\s*(?:\(([^)]*)\))?`)
	reForeignKey  = regexp.MustCompile(`(?is)foreign\s+key\s*\(([^)]*)\)\s*references\s+([^\s(]+)\s*(?:\(([^)]*)\))?`)
	rePrimaryCols = regexp.MustCompile(`(?is)^primary\s+key\s*\(([^)]*)\)`)
	reUniqueCols  = regexp.MustCompile(`(?is)^unique\s*(?:key|index)?\s*\S*\s*\(([^)]*)\)`)
	reConstraint  = regexp.MustCompile(`(?is)^constraint\s+\S+\s+(.*)$`)
)

// applyStatement routes one DDL statement to the right handler.
func applyStatement(b *schemaBuilder, stmt, source string) {
	trimmed := strings.TrimSpace(stmt)
	switch {
	case reCreateTable.MatchString(trimmed):
		parseCreateTable(b, trimmed, source)
	case reUniqueIndex.MatchString(trimmed):
		m := reUniqueIndex.FindStringSubmatch(trimmed)
		tbl := ident(m[1])
		for _, c := range splitList(m[2]) {
			b.markUnique(tbl, ident(c))
		}
	case reAlterTable.MatchString(trimmed):
		m := reAlterTable.FindStringSubmatch(trimmed)
		parseAlterTable(b, ident(m[1]), m[2], source)
	case reDropTable.MatchString(trimmed):
		b.drop(ident(reDropTable.FindStringSubmatch(trimmed)[1]))
	}
}

// parseCreateTable reads the column list plus table-level constraints.
func parseCreateTable(b *schemaBuilder, stmt, source string) {
	m := reCreateTable.FindStringSubmatchIndex(stmt)
	name := ident(stmt[m[2]:m[3]])
	// The match ends on the opening paren of the column list.
	body, ok := parenBody(stmt[m[1]-1:])
	if !ok {
		return
	}
	b.table(name, source)

	var deferred []string // table-level constraints applied after all columns exist
	for _, item := range splitList(body) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if isTableConstraint(item) {
			deferred = append(deferred, item)
			continue
		}
		if col, ok := parseColumn(item); ok {
			b.addColumn(name, source, col)
		}
	}
	for _, item := range deferred {
		applyTableConstraint(b, name, item)
	}
}

// isTableConstraint distinguishes a table-level constraint clause from a column definition.
func isTableConstraint(item string) bool {
	first := strings.ToUpper(firstWord(item))
	switch first {
	case "PRIMARY", "UNIQUE", "FOREIGN", "CONSTRAINT", "CHECK", "EXCLUDE", "INDEX", "KEY", "FULLTEXT", "SPATIAL":
		return true
	}
	return false
}

// applyTableConstraint handles PRIMARY KEY(...), UNIQUE(...), FOREIGN KEY(...) and their CONSTRAINT-named forms.
func applyTableConstraint(b *schemaBuilder, table, item string) {
	item = strings.TrimSpace(item)
	if m := reConstraint.FindStringSubmatch(item); m != nil {
		item = strings.TrimSpace(m[1])
	}
	switch {
	case rePrimaryCols.MatchString(item):
		for _, c := range splitList(rePrimaryCols.FindStringSubmatch(item)[1]) {
			b.markPK(table, ident(c))
		}
	case reForeignKey.MatchString(item):
		m := reForeignKey.FindStringSubmatch(item)
		cols, refCols := splitList(m[1]), splitList(m[3])
		for i, c := range cols {
			refCol := "id"
			if i < len(refCols) {
				refCol = ident(refCols[i])
			}
			b.markRef(table, ident(c), ident(m[2]), refCol)
		}
	case reUniqueCols.MatchString(item):
		cols := splitList(reUniqueCols.FindStringSubmatch(item)[1])
		if len(cols) == 1 { // a composite unique isn't a per-column guarantee
			b.markUnique(table, ident(cols[0]))
		}
	}
}

// parseAlterTable handles the ADD forms that change the schema shape.
func parseAlterTable(b *schemaBuilder, table, rest, source string) {
	for _, clause := range splitList(rest) {
		clause = strings.TrimSpace(clause)
		up := strings.ToUpper(clause)
		switch {
		case strings.HasPrefix(up, "ADD COLUMN "), strings.HasPrefix(up, "ADD "):
			def := strings.TrimSpace(clause[len("ADD"):])
			if strings.HasPrefix(strings.ToUpper(def), "COLUMN ") {
				def = strings.TrimSpace(def[len("COLUMN"):])
			}
			if isTableConstraint(def) {
				applyTableConstraint(b, table, def)
				continue
			}
			if col, ok := parseColumn(def); ok {
				b.addColumn(table, source, col)
			}
		case strings.HasPrefix(up, "DROP COLUMN "):
			b.dropColumn(table, ident(firstWord(clause[len("DROP COLUMN"):])))
		}
	}
}

// columnStop marks where a column's type ends and its constraints begin.
var columnStop = map[string]bool{
	"PRIMARY": true, "NOT": true, "NULL": true, "UNIQUE": true, "DEFAULT": true,
	"REFERENCES": true, "CHECK": true, "GENERATED": true, "AUTO_INCREMENT": true,
	"COLLATE": true, "COMMENT": true, "CONSTRAINT": true,
}

// parseColumn turns "user_id uuid NOT NULL REFERENCES users(id)" into a Column.
func parseColumn(def string) (Column, bool) {
	def = strings.TrimSpace(def)
	name := ident(firstWord(def))
	if name == "" {
		return Column{}, false
	}
	rest := strings.TrimSpace(def[len(firstWord(def)):])

	// Type is everything up to the first constraint keyword.
	var typeParts []string
	fields := splitFields(rest)
	i := 0
	for ; i < len(fields); i++ {
		if columnStop[strings.ToUpper(strings.TrimSuffix(fields[i], ","))] {
			break
		}
		typeParts = append(typeParts, fields[i])
	}
	col := Column{Name: name, Type: strings.Join(typeParts, " ")}

	tail := strings.ToUpper(strings.Join(fields[i:], " "))
	if strings.Contains(tail, "PRIMARY KEY") {
		col.PrimaryKey, col.NotNull = true, true
	}
	if strings.Contains(tail, "NOT NULL") {
		col.NotNull = true
	}
	if strings.Contains(tail, "UNIQUE") {
		col.Unique = true
	}
	if m := reReferences.FindStringSubmatch(strings.Join(fields[i:], " ")); m != nil {
		refCol := "id"
		if cols := splitList(m[2]); len(cols) > 0 && cols[0] != "" {
			refCol = ident(cols[0])
		}
		col.Ref = &Ref{Table: ident(m[1]), Column: refCol}
	}
	return col, true
}

// ─── small string helpers ────────────────────────────────────────────────────

// parenBody returns the contents of the parenthesized group starting at s[0].
func parenBody(s string) (string, bool) {
	if len(s) == 0 || s[0] != '(' {
		return "", false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[1:i], true
			}
		}
	}
	return "", false
}

// splitList splits a comma-separated list, ignoring commas inside parentheses.
func splitList(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
			cur.WriteByte(s[i])
		case ')':
			depth--
			cur.WriteByte(s[i])
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
			cur.WriteByte(s[i])
		default:
			cur.WriteByte(s[i])
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// splitFields splits on whitespace but keeps "(...)" glued to the token before it.
func splitFields(s string) []string {
	raw := strings.Fields(s)
	var out []string
	for _, f := range raw {
		if strings.HasPrefix(f, "(") && len(out) > 0 {
			out[len(out)-1] += f
			continue
		}
		out = append(out, f)
	}
	return out
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\n("); i >= 0 {
		return s[:i]
	}
	return s
}

// ident strips quoting and schema qualification: `"public"."users"` -> users.
func ident(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`\"[]';,")
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return strings.Trim(s, "`\"[]")
}
