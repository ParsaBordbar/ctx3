package db

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// defaultWidth is the terminal width assumed when $COLUMNS is unset.
const defaultWidth = 100

// RenderText renders the full report: detected engines, then the relational schema as boxed tables with inline foreign-key arrows, then the relation list.
func RenderText(r *Report) string {
	var sb strings.Builder
	sb.WriteString("Databases — " + r.Project + "\n\n")

	if len(r.Engines) == 0 {
		sb.WriteString("  No database usage detected.\n")
		return sb.String()
	}

	for _, e := range r.Engines {
		fmt.Fprintf(&sb, "  %-22s %s\n", e.Name, e.Kind)
		for _, ev := range e.Evidence {
			fmt.Fprintf(&sb, "  %-22s   %s\n", "", ev)
		}
	}

	if r.Schema == nil || len(r.Schema.Tables) == 0 {
		if r.Relational() {
			sb.WriteString("\n  No schema found (no SQL migrations, schema.sql, schema.prisma or ORM structs).\n")
		}
		return sb.String()
	}

	sb.WriteString("\nSchema (" + strings.Join(r.Schema.Sources, ", ") + ")\n\n")
	sb.WriteString(renderBoxes(r.Schema, termWidth()))

	if len(r.Schema.Relations) > 0 {
		sb.WriteString("\nRelations\n")
		w := 0
		for _, rel := range r.Schema.Relations {
			if n := len(rel.FromTable + "." + rel.FromColumn); n > w {
				w = n
			}
		}
		for _, rel := range r.Schema.Relations {
			from := rel.FromTable + "." + rel.FromColumn
			to := rel.ToTable + "." + rel.ToColumn
			fmt.Fprintf(&sb, "  %-*s  →  %s  (%s)\n", w, from, to, rel.Cardinality)
		}
	}
	return sb.String()
}

// renderBoxes draws one bordered box per table, packed into rows that fit width.
func renderBoxes(s *Schema, width int) string {
	boxes := make([][]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		boxes = append(boxes, tableBox(t))
	}

	const gap = 3
	var sb strings.Builder
	for i := 0; i < len(boxes); {
		// Greedily fill a row.
		row := []([]string){boxes[i]}
		used := boxWidth(boxes[i])
		j := i + 1
		for j < len(boxes) {
			w := boxWidth(boxes[j])
			if used+gap+w > width {
				break
			}
			row = append(row, boxes[j])
			used += gap + w
			j++
		}
		sb.WriteString(joinBoxes(row, gap))
		if j < len(boxes) {
			sb.WriteByte('\n') // blank line between rows of boxes
		}
		i = j
	}
	return sb.String()
}

// tableBox renders one table as a bordered box, with the table name in the top border and FK columns annotated with their target.
func tableBox(t Table) []string {
	type row struct{ name, typ, flags string }
	rows := make([]row, 0, len(t.Columns))
	nameW, typeW, flagW := 0, 0, 0

	for _, c := range t.Columns {
		var flags []string
		switch {
		case c.PrimaryKey:
			flags = append(flags, "PK")
		case c.Unique:
			flags = append(flags, "UQ")
		case c.NotNull:
			flags = append(flags, "NN")
		}
		if c.Ref != nil {
			flags = append(flags, "FK→"+c.Ref.Table+"."+c.Ref.Column)
		}
		r := row{name: c.Name, typ: c.Type, flags: strings.Join(flags, " ")}
		nameW, typeW, flagW = max(nameW, len([]rune(r.name))), max(typeW, len([]rune(r.typ))), max(flagW, len([]rune(r.flags)))
		rows = append(rows, r)
	}

	inner := nameW + 2 + typeW
	if flagW > 0 {
		inner += 2 + flagW
	}
	title := " " + t.Name + " "
	if n := len([]rune(title)) + 1; n > inner {
		inner = n
	}

	out := []string{"╭─" + title + strings.Repeat("─", inner+1-len([]rune(title))) + "╮"}
	if len(rows) == 0 {
		out = append(out, "│ "+padTo("(no columns parsed)", inner)+" │")
	}
	for _, r := range rows {
		line := padTo(r.name, nameW) + "  " + padTo(r.typ, typeW)
		if flagW > 0 {
			line += "  " + padTo(r.flags, flagW)
		}
		out = append(out, "│ "+padTo(line, inner)+" │")
	}
	out = append(out, "╰"+strings.Repeat("─", inner+2)+"╯")
	return out
}

// joinBoxes places boxes side by side, top-aligned, separated by gap spaces.
func joinBoxes(boxes [][]string, gap int) string {
	height := 0
	for _, b := range boxes {
		height = max(height, len(b))
	}
	sep := strings.Repeat(" ", gap)

	var sb strings.Builder
	for line := 0; line < height; line++ {
		var parts []string
		for _, b := range boxes {
			w := boxWidth(b)
			if line < len(b) {
				parts = append(parts, padTo(b[line], w))
			} else {
				parts = append(parts, strings.Repeat(" ", w))
			}
		}
		sb.WriteString(strings.TrimRight(strings.Join(parts, sep), " "))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func boxWidth(box []string) int {
	w := 0
	for _, l := range box {
		w = max(w, len([]rune(l)))
	}
	return w
}

func padTo(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// termWidth reads $COLUMNS, falling back to a sane default so output is stable in pipes and CI.
func termWidth() int {
	if v, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && v >= 40 {
		return v
	}
	return defaultWidth
}

// RenderMermaid renders the schema as a Mermaid ER diagram (and the engine list as a comment header), suitable for pasting into markdown.
func RenderMermaid(r *Report) string {
	var sb strings.Builder
	sb.WriteString("```mermaid\nerDiagram\n")
	if r.Schema == nil || len(r.Schema.Tables) == 0 {
		sb.WriteString("    %% no relational schema found\n```\n")
		return sb.String()
	}

	for _, rel := range r.Schema.Relations {
		conn := "||--o{"
		if rel.Cardinality == "one-to-one" {
			conn = "||--||"
		}
		fmt.Fprintf(&sb, "    %s %s %s : \"%s\"\n",
			mermaidIdent(rel.ToTable), conn, mermaidIdent(rel.FromTable), rel.FromColumn)
	}
	for _, t := range r.Schema.Tables {
		fmt.Fprintf(&sb, "    %s {\n", mermaidIdent(t.Name))
		for _, c := range t.Columns {
			key := ""
			switch {
			case c.PrimaryKey:
				key = " PK"
			case c.Ref != nil:
				key = " FK"
			case c.Unique:
				key = " UK"
			}
			fmt.Fprintf(&sb, "        %s %s%s\n", mermaidType(c.Type), mermaidIdent(c.Name), key)
		}
		sb.WriteString("    }\n")
	}
	sb.WriteString("```\n")
	return sb.String()
}

// mermaidIdent strips characters Mermaid's ER parser rejects.
func mermaidIdent(s string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "(", "", ")", "", ",", "")
	return r.Replace(s)
}

// mermaidType keeps a column type to a single token.
func mermaidType(s string) string {
	// Keep size/precision readable: varchar(64) -> varchar_64.
	s = strings.NewReplacer("(", "_", ",", "_", ")", "", " ", "_").Replace(strings.TrimSpace(s))
	s = mermaidIdent(s)
	if s == "" {
		return "unknown"
	}
	return s
}
