package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"unicode"
)

// goStruct is a candidate model: a Go struct carrying ORM tags.
type goStruct struct {
	name   string
	table  string
	source string
	fields []goField
}

type goField struct {
	name     string // column name
	goName   string // Go field name (used for FK inference)
	typeName string
	pk       bool
	unique   bool
	notNull  bool
	refTable string // set when a gorm tag names the FK target explicitly
	refCol   string
}

// ormTagKeys are the struct-tag keys that mark a struct as a persisted model.
var ormTagKeys = []string{"gorm", "bun", "db", "sql"}

// parseGoStructs folds ORM-tagged Go structs into the builder.
func parseGoStructs(root string, b *schemaBuilder) {
	var structs []goStruct
	fset := token.NewFileSet()

	walkFiles(root, func(path, rel string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			if s, ok := parseGoStruct(ts.Name.Name, st, rel); ok {
				structs = append(structs, s)
			}
			return true
		})
	})
	if len(structs) == 0 {
		return
	}

	tableOf := map[string]string{}
	for _, s := range structs {
		tableOf[s.name] = s.table
		b.addSource(s.source)
	}

	for _, s := range structs {
		b.table(s.table, s.source)
		for _, f := range s.fields {
			col := Column{Name: f.name, Type: f.typeName, PrimaryKey: f.pk, Unique: f.unique, NotNull: f.notNull || f.pk}
			switch {
			case f.refTable != "":
				refCol := f.refCol
				if refCol == "" {
					refCol = "id"
				}
				col.Ref = &Ref{Table: f.refTable, Column: refCol}
			default:
				if target, ok := inferFK(f.goName, tableOf); ok {
					col.Ref = &Ref{Table: target, Column: "id"}
				}
			}
			b.addColumn(s.table, s.source, col)
		}
	}
}

// parseGoStruct returns the model for a struct, or ok=false when no field carries an ORM tag (i.e. it is not persisted).
func parseGoStruct(name string, st *ast.StructType, source string) (goStruct, bool) {
	s := goStruct{name: name, table: tableName(name), source: source}
	tagged := false

	for _, field := range st.Fields.List {
		tag := ""
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		st := reflect.StructTag(tag)

		// gorm.Model / bun.BaseModel embeds.
		if len(field.Names) == 0 {
			embedded := exprString(field.Type)
			if embedded == "gorm.Model" {
				tagged = true
				s.fields = append(s.fields,
					goField{name: "id", goName: "ID", typeName: "uint", pk: true, notNull: true},
					goField{name: "created_at", goName: "CreatedAt", typeName: "time.Time"},
					goField{name: "updated_at", goName: "UpdatedAt", typeName: "time.Time"},
					goField{name: "deleted_at", goName: "DeletedAt", typeName: "gorm.DeletedAt"},
				)
			}
			if t := bunTable(st.Get("bun")); t != "" {
				tagged, s.table = true, t
			}
			continue
		}

		gorm, hasGorm := st.Lookup("gorm")
		if hasGorm {
			tagged = true
		}
		for _, k := range ormTagKeys {
			if _, ok := st.Lookup(k); ok {
				tagged = true
			}
		}
		if strings.Contains(gorm, "-") && strings.TrimSpace(gorm) == "-" {
			continue // explicitly not persisted
		}

		for _, ident := range field.Names {
			if !ident.IsExported() {
				continue
			}
			f := goField{goName: ident.Name, name: snake(ident.Name), typeName: exprString(field.Type)}
			applyGormTag(&f, gorm)
			if n := tagColumn(st, "db", "sql", "bun"); n != "" && !hasGorm {
				f.name = n
			}
			if f.goName == "ID" && !f.pk {
				f.pk, f.notNull = true, true
			}
			if strings.HasPrefix(f.typeName, "*") {
				f.notNull = false
			}
			// Association fields (User, []Order) are navigation, not columns.
			if isAssociation(f.typeName) {
				continue
			}
			s.fields = append(s.fields, f)
		}
	}
	if !tagged || len(s.fields) == 0 {
		return goStruct{}, false
	}
	return s, true
}

// applyGormTag reads the gorm tag options that describe the column.
func applyGormTag(f *goField, tag string) {
	for _, opt := range strings.Split(tag, ";") {
		opt = strings.TrimSpace(opt)
		low := strings.ToLower(opt)
		key, val, _ := strings.Cut(opt, ":")
		switch {
		case low == "primarykey", low == "primary_key":
			f.pk, f.notNull = true, true
		case low == "unique", strings.HasPrefix(low, "uniqueindex"):
			f.unique = true
		case low == "not null":
			f.notNull = true
		case strings.EqualFold(key, "column"):
			f.name = val
		case strings.EqualFold(key, "type"):
			f.typeName = val
		case strings.EqualFold(key, "references"):
			f.refCol = snake(val)
		}
	}
}

// tagColumn returns the column name from the first of keys that carries one.
func tagColumn(st reflect.StructTag, keys ...string) string {
	for _, k := range keys {
		v, ok := st.Lookup(k)
		if !ok || v == "" || v == "-" {
			continue
		}
		name, _, _ := strings.Cut(v, ",")
		name = strings.TrimSpace(name)
		if k == "bun" && strings.Contains(name, ":") {
			continue // bun directives like "table:users" aren't column names
		}
		if name != "" {
			return name
		}
	}
	return ""
}

// bunTable reads `bun:"table:users"` off an embedded bun.BaseModel.
func bunTable(tag string) string {
	for _, opt := range strings.Split(tag, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(opt), "table:"); ok {
			return v
		}
	}
	return ""
}

// isAssociation reports whether a Go type is a related model rather than a scalar column: slices, maps, and exported named types from another package.
func isAssociation(t string) bool {
	base := strings.TrimPrefix(t, "*")
	switch {
	case strings.HasPrefix(base, "[]") && base != "[]byte":
		return true
	case strings.HasPrefix(base, "map["):
		return true
	}
	// A plain exported identifier (no package qualifier, no builtin) is another model in this package.
	if strings.Contains(base, ".") || base == "" {
		return false
	}
	r := rune(base[0])
	return unicode.IsUpper(r)
}

// inferFK maps a field like OrderID to the orders table when Order is a model.
func inferFK(goName string, tableOf map[string]string) (string, bool) {
	if goName == "ID" || !strings.HasSuffix(goName, "ID") {
		return "", false
	}
	prefix := strings.TrimSuffix(goName, "ID")
	if t, ok := tableOf[prefix]; ok {
		return t, true
	}
	return "", false
}

// tableName is the ORM convention: snake_case, pluralized.
func tableName(structName string) string { return plural(snake(structName)) }

// snake converts CamelCase to snake_case.
func snake(s string) string {
	var out []rune
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLower := i > 0 && unicode.IsLower(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && (prevLower || nextLower) {
				out = append(out, '_')
			}
			out = append(out, unicode.ToLower(r))
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// plural applies the simple English rules the ORMs use.
func plural(s string) string {
	switch {
	case s == "":
		return s
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(s[len(s)-2]):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

func isVowel(b byte) bool { return strings.IndexByte("aeiou", b) >= 0 }

// exprString renders a type expression compactly (*pkg.T, []T, map[K]V).
func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "?"
	}
}
