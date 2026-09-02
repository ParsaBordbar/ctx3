package db

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write creates dir/name with content, making parent dirs as needed.
func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func analyze(t *testing.T, dir string) *Report {
	t.Helper()
	rep, err := Analyze(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return rep
}

func TestDetect_ScopesSignaturesToManifest(t *testing.T) {
	dir := t.TempDir()
	// "ioredis" must not also register the Python bare-"redis" signature, and
	// package.json must not be scanned with Go signatures.
	write(t, dir, "package.json", `{"dependencies":{"ioredis":"^5","pg":"^8"}}`)

	engines := Detect(dir)
	names := map[string]Kind{}
	for _, e := range engines {
		names[e.Name] = e.Kind
	}
	if names["redis"] != KindKeyValue {
		t.Fatalf("expected redis detected, got %v", names)
	}
	if names["postgres"] != KindRelational {
		t.Fatalf("expected postgres detected, got %v", names)
	}
	for _, e := range engines {
		for _, ev := range e.Evidence {
			if !strings.HasPrefix(ev, "package.json:") {
				t.Fatalf("evidence from an unread manifest: %q", ev)
			}
		}
	}
}

func TestDetect_ComposeAndDSN(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "docker-compose.yml", "services:\n  db:\n    image: postgres:16\n  cache:\n    image: redis:7\n")
	write(t, dir, ".env.example", "DATABASE_URL=postgres://u:p@localhost/db\nMONGO_URL=mongodb://localhost:27017\n")

	var got []string
	for _, e := range Detect(dir) {
		got = append(got, e.Name)
	}
	for _, want := range []string{"postgres", "mongodb", "redis"} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	// Relational engines sort first so the report leads with the schema owner.
	if got[0] != "postgres" {
		t.Fatalf("relational engine should sort first, got %v", got)
	}
}

func TestSQL_FoldsMigrationsAndSkipsDown(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "migrations/0001_init.up.sql", `
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE
);
CREATE TABLE orders (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  total numeric(10,2)
);
CREATE TABLE scratch (id int);
`)
	write(t, dir, "migrations/0002_more.up.sql", `
ALTER TABLE users ADD COLUMN name text;
DROP TABLE scratch;
CREATE TABLE items (
  id bigserial PRIMARY KEY,
  order_id uuid NOT NULL,
  CONSTRAINT fk_order FOREIGN KEY (order_id) REFERENCES orders (id)
);
`)
	// The down migration must not undo the schema.
	write(t, dir, "migrations/0002_more.down.sql", "DROP TABLE items;\nALTER TABLE users DROP COLUMN name;\n")

	s := analyze(t, dir).Schema
	if s == nil {
		t.Fatal("no schema parsed")
	}
	if s.Table("scratch") != nil {
		t.Fatal("DROP TABLE should remove the table from the folded schema")
	}
	users := s.Table("users")
	if users == nil {
		t.Fatal("users table missing")
	}
	if len(users.Columns) != 3 {
		t.Fatalf("users columns = %d, want 3 (id, email, name): %+v", len(users.Columns), users.Columns)
	}
	if !users.Columns[0].PrimaryKey || !users.Columns[1].Unique || !users.Columns[1].NotNull {
		t.Fatalf("column constraints not parsed: %+v", users.Columns)
	}

	orders := s.Table("orders")
	if c := orders.Columns[1]; c.Ref == nil || c.Ref.Table != "users" || c.Ref.Column != "id" {
		t.Fatalf("inline REFERENCES not parsed: %+v", c)
	}
	if c := orders.Columns[2]; c.Type != "numeric(10,2)" {
		t.Fatalf("type with precision mangled: %q", c.Type)
	}
	items := s.Table("items")
	if c := items.Columns[1]; c.Ref == nil || c.Ref.Table != "orders" {
		t.Fatalf("table-level FOREIGN KEY not parsed: %+v", c)
	}

	if len(s.Relations) != 2 {
		t.Fatalf("relations = %d, want 2: %+v", len(s.Relations), s.Relations)
	}
	for _, r := range s.Relations {
		if r.Cardinality != "many-to-one" {
			t.Fatalf("non-unique FK should be many-to-one: %+v", r)
		}
	}
}

func TestSQL_GooseUpSectionOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "db/0001_users.sql", `
-- +goose Up
CREATE TABLE users (id serial PRIMARY KEY);
-- +goose Down
DROP TABLE users;
`)
	s := analyze(t, dir).Schema
	if s == nil || s.Table("users") == nil {
		t.Fatal("goose Up section should be applied and Down ignored")
	}
}

func TestSQL_CommentsAndQuotedIdents(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", `
-- users of the app; the ; here must not split the statement
CREATE TABLE "public"."users" ( /* inline */ "id" uuid PRIMARY KEY );
`)
	s := analyze(t, dir).Schema
	u := s.Table("users")
	if u == nil {
		t.Fatalf("schema-qualified quoted table not parsed: %+v", s)
	}
	if u.Columns[0].Name != "id" {
		t.Fatalf("quoted column name not unwrapped: %+v", u.Columns)
	}
}

func TestSQL_OneToOneCardinality(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", `
CREATE TABLE users (id uuid PRIMARY KEY);
CREATE TABLE profiles (
  id uuid PRIMARY KEY,
  user_id uuid UNIQUE REFERENCES users(id)
);
`)
	s := analyze(t, dir).Schema
	if len(s.Relations) != 1 || s.Relations[0].Cardinality != "one-to-one" {
		t.Fatalf("unique FK should be one-to-one: %+v", s.Relations)
	}
}

func TestPrisma_ModelsMapsAndRelations(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "prisma/schema.prisma", `
datasource db {
  provider = "postgresql"
}

model User {
  id    String @id
  email String @unique
  name  String?
  posts Post[]
  @@map("users")
}

model Post {
  id       Int    @id
  author   User   @relation(fields: [authorId], references: [id])
  authorId String @map("author_id")
  @@map("posts")
}
`)
	s := analyze(t, dir).Schema
	if s.Table("users") == nil || s.Table("posts") == nil {
		t.Fatalf("@@map table names not applied: %+v", s.Tables)
	}
	if c := s.Table("users").Columns; len(c) != 3 {
		t.Fatalf("relation list field should not be a column: %+v", c)
	}
	if name := s.Table("users").Columns[2]; name.NotNull {
		t.Fatalf("optional field (String?) should be nullable: %+v", name)
	}
	posts := s.Table("posts")
	if len(posts.Columns) != 2 {
		t.Fatalf("relation object field should not be a column: %+v", posts.Columns)
	}
	fk := posts.Columns[1]
	if fk.Name != "author_id" || fk.Ref == nil || fk.Ref.Table != "users" || fk.Ref.Column != "id" {
		t.Fatalf("@relation not resolved to the mapped table: %+v", fk)
	}
}

func TestGoStructs_GormTagsAndInferredFK(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module demo\n\ngo 1.24\n\nrequire gorm.io/gorm v1.25.0\n")
	write(t, dir, "models/models.go", "package models\n\nimport \"time\"\n\n"+
		"type User struct {\n"+
		"\tID        uint   `gorm:\"primaryKey\"`\n"+
		"\tEmail     string `gorm:\"unique;not null\"`\n"+
		"\tName      string `gorm:\"column:full_name\"`\n"+
		"\tCreatedAt time.Time\n"+
		"\tOrders    []Order\n"+
		"}\n\n"+
		"type Order struct {\n"+
		"\tID     uint    `gorm:\"primaryKey\"`\n"+
		"\tUserID uint    `gorm:\"not null\"`\n"+
		"\tTotal  float64 `gorm:\"type:numeric(10,2)\"`\n"+
		"\tUser   User\n"+
		"}\n")

	rep := analyze(t, dir)
	s := rep.Schema
	if s == nil {
		t.Fatal("no schema parsed from structs")
	}
	users := s.Table("users")
	if users == nil {
		t.Fatalf("struct name should pluralize to users: %+v", s.Tables)
	}
	if len(users.Columns) != 4 {
		t.Fatalf("association field should not be a column: %+v", users.Columns)
	}
	if users.Columns[2].Name != "full_name" {
		t.Fatalf("gorm column tag ignored: %+v", users.Columns[2])
	}
	if !users.Columns[1].Unique || !users.Columns[1].NotNull {
		t.Fatalf("gorm unique/not null ignored: %+v", users.Columns[1])
	}
	orders := s.Table("orders")
	if fk := orders.Columns[1]; fk.Ref == nil || fk.Ref.Table != "users" {
		t.Fatalf("UserID should infer a FK to users: %+v", fk)
	}
	if orders.Columns[2].Type != "numeric(10,2)" {
		t.Fatalf("gorm type tag ignored: %+v", orders.Columns[2])
	}
}

func TestGoStructs_IgnoresUntaggedStructs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module demo\n\ngo 1.24\n")
	write(t, dir, "types.go", "package demo\n\ntype Config struct {\n\tName string\n\tPort int\n}\n")

	if rep := analyze(t, dir); rep.Schema != nil {
		t.Fatalf("plain structs must not become tables: %+v", rep.Schema.Tables)
	}
}

func TestAnalyze_SchemaImpliesRelationalEngine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", "CREATE TABLE users (id int PRIMARY KEY);")

	rep := analyze(t, dir)
	if !rep.Relational() {
		t.Fatalf("a parsed schema should imply a relational engine: %+v", rep.Engines)
	}
}

func TestAnalyze_EmptyProject(t *testing.T) {
	rep := analyze(t, t.TempDir())
	if len(rep.Engines) != 0 || rep.Schema != nil {
		t.Fatalf("empty project should yield no engines and no schema: %+v", rep)
	}
	if out := RenderText(rep); !strings.Contains(out, "No database usage detected") {
		t.Fatalf("empty report should say so:\n%s", out)
	}
}

func TestRenderText_BoxesAlignAndShowFK(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", `
CREATE TABLE users (id uuid PRIMARY KEY, email text NOT NULL UNIQUE);
CREATE TABLE orders (id uuid PRIMARY KEY, user_id uuid REFERENCES users(id));
`)
	out := RenderText(analyze(t, dir))

	if !strings.Contains(out, "FK→users.id") {
		t.Fatalf("foreign key should be annotated inline:\n%s", out)
	}
	if !strings.Contains(out, "orders.user_id  →  users.id") {
		t.Fatalf("relation list missing:\n%s", out)
	}
	// Every box line in a row must be the same rune width, or the borders skew.
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "╭") && !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "╰") {
			continue
		}
		if strings.Count(line, "╮")+strings.Count(line, "│")+strings.Count(line, "╯") == 0 {
			t.Fatalf("unterminated box line: %q", line)
		}
	}
}

func TestRenderMermaid_ERDiagram(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", `
CREATE TABLE users (id uuid PRIMARY KEY);
CREATE TABLE orders (id uuid PRIMARY KEY, user_id uuid REFERENCES users(id), total numeric(10,2));
`)
	out := RenderMermaid(analyze(t, dir))
	for _, want := range []string{"erDiagram", "users ||--o{ orders", "uuid id PK", "uuid user_id FK", "numeric_10_2 total"} {
		if !strings.Contains(out, want) {
			t.Fatalf("mermaid missing %q:\n%s", want, out)
		}
	}
}

func TestSkill_TriggersAndReferences(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql", "CREATE TABLE users (id uuid PRIMARY KEY);")

	skill := analyze(t, dir).Skill("myproj")
	if skill.Name != "myproj-db" {
		t.Fatalf("skill name should be myproj-db, got %q", skill.Name)
	}
	// An empty name falls back to the project the report carries, so a
	// standalone caller still gets a sensible skill.
	if fallback := analyze(t, dir).Skill(""); !strings.HasSuffix(fallback.Name, "-db") {
		t.Fatalf("fallback name should end in -db: %q", fallback.Name)
	}
	if !strings.Contains(skill.Description, "schema") {
		t.Fatalf("skill description should trigger on schema questions: %q", skill.Description)
	}
	if len(skill.References) != 2 {
		t.Fatalf("want databases.md + schema.mermaid.md, got %d", len(skill.References))
	}
}

func TestAnalyze_Deterministic(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "migrations/0001.up.sql", "CREATE TABLE b (id int PRIMARY KEY);\nCREATE TABLE a (id int PRIMARY KEY, b_id int REFERENCES b(id));")
	first := RenderText(analyze(t, dir))
	for i := 0; i < 3; i++ {
		if got := RenderText(analyze(t, dir)); got != first {
			t.Fatalf("output not deterministic on run %d", i)
		}
	}
	if !strings.Contains(first, "╭─ a ") {
		t.Fatalf("tables should be sorted by name:\n%s", first)
	}
}

func contains(list []string, want string) bool { return slices.Contains(list, want) }
