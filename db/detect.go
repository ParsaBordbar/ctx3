package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// signature maps a substring found in a manifest/config file to the engine it implies.
type signature struct {
	match  string // substring to look for
	engine string
	kind   Kind
}

// manifestSigs maps each dependency manifest to the signatures that are meaningful in it.
var manifestSigs = map[string][]signature{
	"go.mod":           goSigs,
	"package.json":     jsSigs,
	"requirements.txt": pySigs,
	"pyproject.toml":   pySigs,
	"Pipfile":          pySigs,
	"Cargo.toml":       rustSigs,
	"Gemfile":          rubySigs,
	"composer.json":    phpSigs,
	"pom.xml":          jvmSigs,
	"build.gradle":     jvmSigs,
	"build.gradle.kts": jvmSigs,
}

var goSigs = []signature{
	{"github.com/lib/pq", "postgres", KindRelational},
	{"github.com/jackc/pgx", "postgres", KindRelational},
	{"gorm.io/driver/postgres", "postgres", KindRelational},
	{"go-sql-driver/mysql", "mysql", KindRelational},
	{"gorm.io/driver/mysql", "mysql", KindRelational},
	{"mattn/go-sqlite3", "sqlite", KindEmbedded},
	{"modernc.org/sqlite", "sqlite", KindEmbedded},
	{"gorm.io/driver/sqlite", "sqlite", KindEmbedded},
	{"go.mongodb.org/mongo-driver", "mongodb", KindDocument},
	{"redis/go-redis", "redis", KindKeyValue},
	{"gomodule/redigo", "redis", KindKeyValue},
	{"clickhouse-go", "clickhouse", KindColumnar},
	{"gocql/gocql", "cassandra", KindColumnar},
	{"neo4j-go-driver", "neo4j", KindGraph},
	{"elastic/go-elasticsearch", "elasticsearch", KindSearch},
	{"opensearch-go", "opensearch", KindSearch},
	{"etcd-io/bbolt", "bolt", KindEmbedded},
	{"dgraph-io/badger", "badger", KindEmbedded},
	{"qdrant-go", "qdrant", KindVector},
	{"pinecone-io", "pinecone", KindVector},
	{"aws-sdk-go/service/dynamodb", "dynamodb", KindKeyValue},
	{"aws-sdk-go-v2/service/dynamodb", "dynamodb", KindKeyValue},
	{"gorm.io/gorm", "sql (gorm)", KindRelational},
	{"entgo.io/ent", "sql (ent)", KindRelational},
	{"jmoiron/sqlx", "sql (sqlx)", KindRelational},
	{"uptrace/bun", "sql (bun)", KindRelational},
}

var jsSigs = []signature{
	{"\"pg\"", "postgres", KindRelational},
	{"\"postgres\"", "postgres", KindRelational},
	{"\"mysql2\"", "mysql", KindRelational},
	{"\"mysql\"", "mysql", KindRelational},
	{"better-sqlite3", "sqlite", KindEmbedded},
	{"\"sqlite3\"", "sqlite", KindEmbedded},
	{"mongoose", "mongodb", KindDocument},
	{"\"mongodb\"", "mongodb", KindDocument},
	{"\"ioredis\"", "redis", KindKeyValue},
	{"\"redis\"", "redis", KindKeyValue},
	{"@elastic/elasticsearch", "elasticsearch", KindSearch},
	{"\"prisma\"", "sql (prisma)", KindRelational},
	{"@prisma/client", "sql (prisma)", KindRelational},
	{"drizzle-orm", "sql (drizzle)", KindRelational},
	{"typeorm", "sql (typeorm)", KindRelational},
	{"sequelize", "sql (sequelize)", KindRelational},
	{"\"knex\"", "sql (knex)", KindRelational},
}

var pySigs = []signature{
	{"psycopg", "postgres", KindRelational},
	{"asyncpg", "postgres", KindRelational},
	{"mysqlclient", "mysql", KindRelational},
	{"pymysql", "mysql", KindRelational},
	{"pymongo", "mongodb", KindDocument},
	{"motor", "mongodb", KindDocument},
	{"redis", "redis", KindKeyValue},
	{"sqlalchemy", "sql (sqlalchemy)", KindRelational},
	{"django", "sql (django orm)", KindRelational},
	{"elasticsearch", "elasticsearch", KindSearch},
}

var rustSigs = []signature{
	{"rusqlite", "sqlite", KindEmbedded},
	{"tokio-postgres", "postgres", KindRelational},
	{"mongodb", "mongodb", KindDocument},
	{"redis", "redis", KindKeyValue},
	{"diesel", "sql (diesel)", KindRelational},
	{"sqlx", "sql (sqlx)", KindRelational},
	{"sea-orm", "sql (sea-orm)", KindRelational},
}

var rubySigs = []signature{
	{"pg", "postgres", KindRelational},
	{"mysql2", "mysql", KindRelational},
	{"sqlite3", "sqlite", KindEmbedded},
	{"mongoid", "mongodb", KindDocument},
	{"redis", "redis", KindKeyValue},
	{"activerecord", "sql (activerecord)", KindRelational},
}

var phpSigs = []signature{
	{"doctrine/orm", "sql (doctrine)", KindRelational},
	{"illuminate/database", "sql (eloquent)", KindRelational},
	{"predis/predis", "redis", KindKeyValue},
	{"mongodb/mongodb", "mongodb", KindDocument},
}

var jvmSigs = []signature{
	{"org.postgresql", "postgres", KindRelational},
	{"mysql-connector", "mysql", KindRelational},
	{"org.xerial:sqlite", "sqlite", KindEmbedded},
	{"mongodb-driver", "mongodb", KindDocument},
	{"jedis", "redis", KindKeyValue},
	{"lettuce", "redis", KindKeyValue},
	{"hibernate", "sql (hibernate)", KindRelational},
	{"spring-boot-starter-data-jpa", "sql (jpa)", KindRelational},
}

// imageSigs maps a container image name to its engine.
var imageSigs = []signature{
	{"postgres", "postgres", KindRelational},
	{"timescale", "timescaledb", KindRelational},
	{"pgvector", "postgres (pgvector)", KindVector},
	{"mariadb", "mariadb", KindRelational},
	{"mysql", "mysql", KindRelational},
	{"cockroach", "cockroachdb", KindRelational},
	{"mongo", "mongodb", KindDocument},
	{"redis", "redis", KindKeyValue},
	{"valkey", "valkey", KindKeyValue},
	{"clickhouse", "clickhouse", KindColumnar},
	{"cassandra", "cassandra", KindColumnar},
	{"scylla", "scylladb", KindColumnar},
	{"elasticsearch", "elasticsearch", KindSearch},
	{"opensearch", "opensearch", KindSearch},
	{"meilisearch", "meilisearch", KindSearch},
	{"neo4j", "neo4j", KindGraph},
	{"qdrant", "qdrant", KindVector},
	{"weaviate", "weaviate", KindVector},
	{"chroma", "chroma", KindVector},
	{"minio", "minio", KindKeyValue},
	{"etcd", "etcd", KindKeyValue},
}

// dsnSchemes maps a connection-string scheme to its engine.
var dsnSchemes = []signature{
	{"postgres://", "postgres", KindRelational},
	{"postgresql://", "postgres", KindRelational},
	{"mysql://", "mysql", KindRelational},
	{"sqlite://", "sqlite", KindEmbedded},
	{"file:./", "sqlite", KindEmbedded},
	{"mongodb://", "mongodb", KindDocument},
	{"mongodb+srv://", "mongodb", KindDocument},
	{"redis://", "redis", KindKeyValue},
	{"rediss://", "redis", KindKeyValue},
	{"clickhouse://", "clickhouse", KindColumnar},
	{"neo4j://", "neo4j", KindGraph},
	{"bolt://", "neo4j", KindGraph},
}

// envFiles are configuration files scanned for DSNs.
var envFiles = []string{
	".env", ".env.example", ".env.sample", ".env.local", ".env.template",
	"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml",
}

var imageLine = regexp.MustCompile(`(?i)^\s*image:\s*["']?([\w./-]+)`)

// Detect returns every database the project appears to use, sorted by name.
func Detect(root string) []Engine {
	acc := map[string]*Engine{}

	add := func(name string, kind Kind, evidence string) {
		e, ok := acc[name]
		if !ok {
			e = &Engine{Name: name, Kind: kind}
			acc[name] = e
		}
		e.Evidence = append(e.Evidence, evidence)
	}

	// 1. Dependency manifests — drivers and ORMs.
	manifestNames := make([]string, 0, len(manifestSigs))
	for m := range manifestSigs {
		manifestNames = append(manifestNames, m)
	}
	sort.Strings(manifestNames)
	for _, m := range manifestNames {
		data, err := os.ReadFile(filepath.Join(root, m))
		if err != nil {
			continue
		}
		low := strings.ToLower(string(data))
		for _, s := range manifestSigs[m] {
			if strings.Contains(low, s.match) {
				add(s.engine, s.kind, m+": "+strings.Trim(s.match, `"`))
			}
		}
	}

	// 2.
	for _, f := range envFiles {
		path := filepath.Join(root, f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := imageLine.FindStringSubmatch(line); m != nil {
				img := strings.ToLower(m[1])
				for _, s := range imageSigs {
					if strings.Contains(img, s.match) {
						add(s.engine, s.kind, f+": image "+m[1])
						break // first (most specific) match wins
					}
				}
			}
			low := strings.ToLower(line)
			for _, s := range dsnSchemes {
				if strings.Contains(low, s.match) {
					add(s.engine, s.kind, f+": "+s.match)
				}
			}
		}
	}

	// 4. Committed embedded database files.
	walkFiles(root, func(path, rel string) {
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".db", ".sqlite", ".sqlite3":
			add("sqlite", KindEmbedded, "file: "+rel)
		}
	})

	engines := make([]Engine, 0, len(acc))
	for _, e := range acc {
		e.Evidence = sortedUnique(e.Evidence)
		engines = append(engines, *e)
	}
	sortEngines(engines)
	return engines
}

// sortEngines orders relational stores first (they carry the schema), then by name.
func sortEngines(engines []Engine) {
	sort.SliceStable(engines, func(i, j int) bool {
		ri, rj := engines[i].Kind == KindRelational, engines[j].Kind == KindRelational
		if ri != rj {
			return ri
		}
		return engines[i].Name < engines[j].Name
	})
}
