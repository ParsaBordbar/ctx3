package db

import (
	"os"
	"regexp"
	"strings"
)

var (
	reModelOpen  = regexp.MustCompile(`(?i)^\s*model\s+(\w+)\s*\{`)
	reMapAttr    = regexp.MustCompile(`(?i)@@map\("([^"]+)"\)`)
	reBlockID    = regexp.MustCompile(`(?i)@@id\(\s*\[([^\]]*)\]`)
	reBlockUniq  = regexp.MustCompile(`(?i)@@unique\(\s*\[([^\]]*)\]`)
	reFieldMap   = regexp.MustCompile(`(?i)@map\("([^"]+)"\)`)
	reRelation   = regexp.MustCompile(`(?i)@relation\((.*)\)`)
	reRelFields  = regexp.MustCompile(`(?i)fields:\s*\[([^\]]*)\]`)
	reRelRefs    = regexp.MustCompile(`(?i)references:\s*\[([^\]]*)\]`)
	prismaIgnore = regexp.MustCompile(`(?i)^\s*(datasource|generator|enum)\s`)
)

// prismaModel is one parsed `model` block, kept raw until every model is known so a @relation can be resolved to the target model's mapped table name.
type prismaModel struct {
	name   string // model name as written
	table  string // @@map value, else model name
	fields []prismaField
	blockU []string // @@unique single-column lists
	blockP []string // @@id columns
}

type prismaField struct {
	name     string // column name after @map
	prismaN  string // field name as written (for @relation fields: [...])
	typeName string
	isID     bool
	isUnique bool
	optional bool
	list     bool
	relFrom  []string // @relation fields: [...]
	relTo    []string // @relation references: [...]
	relModel string   // target model when this field is a relation object
}

// parsePrisma folds every schema.prisma in the repo into the builder.
func parsePrisma(root string, b *schemaBuilder) {
	walkFiles(root, func(path, rel string) {
		if !strings.HasSuffix(strings.ToLower(rel), ".prisma") {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		models := parsePrismaModels(string(data))
		if len(models) == 0 {
			return
		}
		b.addSource(rel)
		applyPrismaModels(b, models, rel)
	})
}

// parsePrismaModels extracts the model blocks.
func parsePrismaModels(src string) []prismaModel {
	var models []prismaModel
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if prismaIgnore.MatchString(line) {
			for ; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "}"); i++ {
			}
			continue
		}
		m := reModelOpen.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		model := prismaModel{name: m[1], table: m[1]}
		for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "}"); i++ {
			parsePrismaLine(&model, strings.TrimSpace(lines[i]))
		}
		models = append(models, model)
	}
	return models
}

func parsePrismaLine(model *prismaModel, line string) {
	if line == "" || strings.HasPrefix(line, "//") {
		return
	}
	if strings.HasPrefix(line, "@@") {
		if m := reMapAttr.FindStringSubmatch(line); m != nil {
			model.table = m[1]
		}
		if m := reBlockID.FindStringSubmatch(line); m != nil {
			model.blockP = append(model.blockP, splitCSV(m[1])...)
		}
		if m := reBlockUniq.FindStringSubmatch(line); m != nil {
			if cols := splitCSV(m[1]); len(cols) == 1 {
				model.blockU = append(model.blockU, cols[0])
			}
		}
		return
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	f := prismaField{prismaN: fields[0], name: fields[0], typeName: fields[1]}
	f.optional = strings.HasSuffix(f.typeName, "?")
	f.list = strings.HasSuffix(f.typeName, "[]")
	f.typeName = strings.TrimSuffix(strings.TrimSuffix(f.typeName, "?"), "[]")

	attrs := strings.Join(fields[2:], " ")
	f.isID = strings.Contains(attrs, "@id")
	f.isUnique = strings.Contains(attrs, "@unique")
	if m := reFieldMap.FindStringSubmatch(attrs); m != nil {
		f.name = m[1]
	}
	if m := reRelation.FindStringSubmatch(attrs); m != nil {
		f.relModel = f.typeName
		if fm := reRelFields.FindStringSubmatch(m[1]); fm != nil {
			f.relFrom = splitCSV(fm[1])
		}
		if rm := reRelRefs.FindStringSubmatch(m[1]); rm != nil {
			f.relTo = splitCSV(rm[1])
		}
	}
	model.fields = append(model.fields, f)
}

// applyPrismaModels writes scalar fields as columns, then resolves relations to the mapped table names.
func applyPrismaModels(b *schemaBuilder, models []prismaModel, source string) {
	tableOf := map[string]string{}          // model name -> table name
	colOf := map[string]map[string]string{} // model -> prisma field -> column name
	scalar := map[string]bool{}
	for _, m := range models {
		tableOf[m.name] = m.table
		colOf[m.name] = map[string]string{}
		for _, f := range m.fields {
			colOf[m.name][f.prismaN] = f.name
			if f.relModel == "" && !f.list {
				scalar[m.name+"."+f.prismaN] = true
			}
		}
	}

	for _, m := range models {
		b.table(m.table, source)
		for _, f := range m.fields {
			if f.relModel != "" || f.list || !scalar[m.name+"."+f.prismaN] {
				continue // relation object or back-reference list, not a column
			}
			b.addColumn(m.table, source, Column{
				Name:       f.name,
				Type:       f.typeName,
				PrimaryKey: f.isID,
				Unique:     f.isUnique,
				NotNull:    !f.optional,
			})
		}
		for _, c := range m.blockP {
			b.markPK(m.table, colOf[m.name][c])
		}
		for _, c := range m.blockU {
			b.markUnique(m.table, colOf[m.name][c])
		}
	}

	// Second pass: @relation(fields: [authorId], references: [id]).
	for _, m := range models {
		for _, f := range m.fields {
			if f.relModel == "" || len(f.relFrom) == 0 {
				continue
			}
			target, ok := tableOf[f.relModel]
			if !ok {
				continue
			}
			for i, from := range f.relFrom {
				to := "id"
				if i < len(f.relTo) {
					to = f.relTo[i]
					if mapped, ok := colOf[f.relModel][to]; ok && mapped != "" {
						to = mapped
					}
				}
				col := from
				if mapped, ok := colOf[m.name][from]; ok && mapped != "" {
					col = mapped
				}
				b.markRef(m.table, col, target, to)
			}
		}
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
