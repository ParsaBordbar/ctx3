package symbols

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const sample = `package sample

// Kind is a classification.
type Kind string

const (
	// KindA is the first.
	KindA Kind = "a"
	kindB Kind = "b"
)

var Registry = map[string]Kind{}

// Store holds records.
type Store struct {
	Name string
	n    int
}

// Reader reads.
type Reader interface {
	Read(p []byte) (int, error)
}

type Alias = Store

// New builds a Store.
func New(name string) *Store { return &Store{Name: name} }

func (s *Store) Add(k Kind, v int) error { return nil }

func helper() {}
`

func writeSample(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func find(idx *Index, name string) *Symbol {
	for i := range idx.Symbols {
		if idx.Symbols[i].Name == name {
			return &idx.Symbols[i]
		}
	}
	return nil
}

func TestScan_KindsAndSignatures(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		kind Kind
		sig  string
	}{
		"Kind":     {KindType, "type Kind string"},
		"KindA":    {KindConst, "const KindA Kind"},
		"Registry": {KindVar, "var Registry = map[string]Kind{}"},
		"Store":    {KindStruct, "type Store struct"},
		"Reader":   {KindInterface, "type Reader interface"},
		"Alias":    {KindType, "type Alias = Store"},
		"New":      {KindFunc, "func New(name string) *Store"},
		"Add":      {KindMethod, "func (s *Store) Add(k Kind, v int) error"},
	}
	for name, w := range want {
		got := find(idx, name)
		if got == nil {
			t.Fatalf("%s missing from index", name)
		}
		if got.Kind != w.kind {
			t.Errorf("%s kind = %q, want %q", name, got.Kind, w.kind)
		}
		if got.Signature != w.sig {
			t.Errorf("%s signature = %q, want %q", name, got.Signature, w.sig)
		}
	}
}

func TestScan_ExportedOnlyByDefault(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, unexported := range []string{"helper", "kindB"} {
		if find(idx, unexported) != nil {
			t.Errorf("%s should be dropped without IncludeUnexported", unexported)
		}
	}

	all, err := Scan(Config{Path: dir, IncludeUnexported: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, unexported := range []string{"helper", "kindB"} {
		if find(all, unexported) == nil {
			t.Errorf("%s missing with IncludeUnexported", unexported)
		}
	}
}

func TestScan_MembersAndDocs(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	idx, err := Scan(Config{Path: dir, Members: true, IncludeUnexported: true})
	if err != nil {
		t.Fatal(err)
	}

	store := find(idx, "Store")
	if store.Doc != "Store holds records." {
		t.Errorf("Store doc = %q", store.Doc)
	}
	if len(store.Members) != 2 || store.Members[0].Name != "Name" || store.Members[1].Type != "int" {
		t.Errorf("Store members = %+v", store.Members)
	}

	reader := find(idx, "Reader")
	if len(reader.Members) != 1 || reader.Members[0].Type != "Read(p []byte) (int, error)" {
		t.Errorf("Reader members = %+v", reader.Members)
	}
}

func TestScan_KindAndMatchFilters(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)

	idx, err := Scan(Config{Path: dir, Kinds: []Kind{KindStruct, KindInterface}})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Symbols) != 2 {
		t.Fatalf("kind filter kept %d symbols, want 2", len(idx.Symbols))
	}

	idx, err = Scan(Config{Path: dir, Match: regexp.MustCompile("^Kind")})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range idx.Symbols {
		if !strings.HasPrefix(s.Name, "Kind") {
			t.Errorf("match filter kept %q", s.Name)
		}
	}
}

func TestScan_SkipsUnparsableFiles(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package sample\nfunc ("), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatalf("a broken file should not fail the scan: %v", err)
	}
	if find(idx, "New") == nil {
		t.Error("good file dropped alongside the broken one")
	}
}

func TestScan_TestsExcludedByDefault(t *testing.T) {
	dir := writeSample(t, "sample_test.go", "package sample\n\nfunc TestThing() {}\n")
	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Symbols) != 0 {
		t.Fatalf("_test.go indexed by default: %+v", idx.Symbols)
	}

	idx, err = Scan(Config{Path: dir, IncludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if find(idx, "TestThing") == nil {
		t.Error("TestThing missing with IncludeTests")
	}
}

func TestRenderGrep_IsFileLineSignature(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	idx, err := Scan(Config{Path: dir, Kinds: []Kind{KindFunc}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(RenderGrep(idx))
	want := filepath.ToSlash(filepath.Join(dir, "sample.go")) + ":28: func New(name string) *Store"
	if got != want {
		t.Errorf("RenderGrep = %q, want %q", got, want)
	}
}

func TestCounts_OrderedByKind(t *testing.T) {
	dir := writeSample(t, "sample.go", sample)
	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(Counts(idx), " ")
	want := "1 const 1 var 1 struct 1 interface 2 type 1 func 1 method"
	if got != want {
		t.Errorf("Counts = %q, want %q", got, want)
	}
}
