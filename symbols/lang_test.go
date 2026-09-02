package symbols

import (
	"os"
	"path/filepath"
	"testing"
)

// index is a scan result, searched by name and kind rather than by name alone:
// a Java constructor and its class legitimately share a name.
type index []Symbol

// scanSource writes one source file and indexes it, unexported included.
func scanSource(t *testing.T, name, body string) index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := Scan(Config{Path: dir, IncludeUnexported: true})
	if err != nil {
		t.Fatal(err)
	}
	return idx.Symbols
}

// find returns the first symbol with this name and kind.
func (in index) find(name string, kind Kind) (Symbol, bool) {
	for _, s := range in {
		if s.Name == name && s.Kind == kind {
			return s, true
		}
	}
	return Symbol{}, false
}

// has reports whether any symbol carries this name, whatever its kind.
func (in index) has(name string) bool {
	for _, s := range in {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (in index) names() []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, string(s.Kind)+" "+s.Name)
	}
	return out
}

func want(t *testing.T, got index, name string, kind Kind, recv string) Symbol {
	t.Helper()
	s, ok := got.find(name, kind)
	if !ok {
		t.Errorf("no %s named %q (have %v)", kind, name, got.names())
		return Symbol{}
	}
	if s.Recv != recv {
		t.Errorf("%s %s recv = %q, want %q", kind, name, s.Recv, recv)
	}
	return s
}

func TestScanTypeScript(t *testing.T) {
	got := scanSource(t, "api.ts", `
// UserService talks to the API.
export class UserService {
  async fetchUser(id: string): Promise<User> {
    if (this.cache.has(id)) {
      return this.cache.get(id);
    }
    return get(id);
  }
}

export interface User { id: string }
export type UserId = string;
export const DEFAULT_TIMEOUT = 30;
export const makeClient = (url: string) => new UserService();
function helper(x: number) { return x + 1; }
`)

	want(t, got, "UserService", KindStruct, "")
	want(t, got, "fetchUser", KindMethod, "UserService")
	want(t, got, "User", KindInterface, "")
	want(t, got, "UserId", KindType, "")
	want(t, got, "DEFAULT_TIMEOUT", KindConst, "")
	// An arrow-function binding is a function, not a const.
	want(t, got, "makeClient", KindFunc, "")
	want(t, got, "helper", KindFunc, "")

	if doc := want(t, got, "UserService", KindStruct, "").Doc; doc != "UserService talks to the API." {
		t.Errorf("doc = %q", doc)
	}
	// `if (...)` inside a class body must not read as a method declaration.
	if got.has("if") {
		t.Error("control-flow keyword indexed as a method")
	}
}

func TestScanPython(t *testing.T) {
	got := scanSource(t, "model.py", `
import os

MAX_RETRIES = 5
_private = 1

class Repository:
    """Stores users."""

    def __init__(self, dsn):
        self.dsn = dsn

    # find_user looks up a user.
    def find_user(self, uid):
        return None

    async def save(self, user):
        pass

def connect(dsn):
    return Repository(dsn)
`)

	want(t, got, "MAX_RETRIES", KindConst, "")
	want(t, got, "_private", KindVar, "")
	want(t, got, "Repository", KindStruct, "")
	// Blank lines and docstrings inside a class must not close its scope.
	want(t, got, "__init__", KindMethod, "Repository")
	want(t, got, "find_user", KindMethod, "Repository")
	want(t, got, "save", KindMethod, "Repository")
	// A def back at column 0 is module level again.
	want(t, got, "connect", KindFunc, "")

	if doc := want(t, got, "find_user", KindMethod, "Repository").Doc; doc != "find_user looks up a user." {
		t.Errorf("doc = %q", doc)
	}
}

func TestScanRust(t *testing.T) {
	got := scanSource(t, "lib.rs", `
pub struct Config { pub port: u16 }

pub trait Handler {
    fn handle(&self) -> u32;
}

impl Config {
    pub fn new(port: u16) -> Self { Config { port } }
}

pub const VERSION: &str = "1.0";
pub enum Mode { Fast, Slow }
fn internal_helper() {}
`)

	want(t, got, "Config", KindStruct, "")
	want(t, got, "Handler", KindInterface, "")
	// `impl Config` scopes methods to the type, though it emits no symbol itself.
	want(t, got, "new", KindMethod, "Config")
	want(t, got, "handle", KindMethod, "Handler")
	want(t, got, "VERSION", KindConst, "")
	want(t, got, "Mode", KindType, "")
	want(t, got, "internal_helper", KindFunc, "")
}

func TestScanRuby(t *testing.T) {
	got := scanSource(t, "user.rb", `
class User
  MAX = 10

  def name
    @name
  end

  def self.build(attrs)
    new(attrs)
  end
end

module Helpers
  def slugify(s)
  end
end
`)

	want(t, got, "User", KindStruct, "")
	want(t, got, "name", KindMethod, "User")
	want(t, got, "build", KindMethod, "User")
	want(t, got, "Helpers", KindType, "")
	want(t, got, "slugify", KindMethod, "Helpers")
}

func TestScanJava(t *testing.T) {
	got := scanSource(t, "Service.java", `
public class Service {
    private final Repo repo;

    public Service(Repo repo) {
        this.repo = repo;
    }

    public List<User> findAll(int limit) {
        return repo.all(limit);
    }
}

interface Repo {
}
`)

	want(t, got, "Service", KindStruct, "")
	want(t, got, "findAll", KindMethod, "Service")
	want(t, got, "Repo", KindInterface, "")
	// A constructor shares its class's name and is indexed as a method.
	want(t, got, "Service", KindMethod, "Service")
	// `this.repo = repo;` is a statement, not a declaration.
	if got.has("repo") {
		t.Error("field assignment indexed as a declaration")
	}
}

func TestScanUnexportedFiltering(t *testing.T) {
	dir := t.TempDir()
	src := "pub fn Public() {}\nfn private_one() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "x.rs"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Scan(Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Symbols) != 1 || idx.Symbols[0].Name != "Public" {
		t.Fatalf("exported-only scan kept %v", idx.Symbols)
	}
}

func TestScanLangFilter(t *testing.T) {
	dir := t.TempDir()
	writes := map[string]string{
		"a.py": "def only_python():\n    pass\n",
		"b.rs": "pub fn only_rust() {}\n",
	}
	for name, body := range writes {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	idx, err := Scan(Config{Path: dir, IncludeUnexported: true, Langs: []string{"rust"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Symbols) != 1 || idx.Symbols[0].Name != "only_rust" {
		t.Fatalf("--lang rust kept %v", idx.Symbols)
	}
	if idx.Symbols[0].Lang != "rust" {
		t.Errorf("lang = %q", idx.Symbols[0].Lang)
	}
}

func TestScanGoStillTaggedGo(t *testing.T) {
	got := scanSource(t, "x.go", "package p\n\n// Do does a thing.\nfunc Do() {}\n")
	if s := want(t, got, "Do", KindFunc, ""); s.Lang != "go" {
		t.Fatalf("go symbol lang = %q", s.Lang)
	}
}
