package pack

import "runtime"

type OutputFormat string

const (
	FormatXML OutputFormat = "xml"
	FormatMD  OutputFormat = "md"
	FormatTXT OutputFormat = "txt"
)

type BinaryStrategy string

const (
	BinarySkip   BinaryStrategy = "skip"
	BinaryHex    BinaryStrategy = "hex"
	BinaryBase64 BinaryStrategy = "base64"
)

type Sections struct {
	Structure bool
	Files     bool
}

type Config struct {
	RootDir          string
	OutputFormat     OutputFormat
	OutputPath       string
	RespectGitignore bool
	IncludeGlobs     []string
	IgnoreGlobs      []string
	MaxFileBytes     int64
	MaxTotalBytes    int64
	BinaryHandling   BinaryStrategy
	SortByExt        bool // false = sort by path
	Sections         Sections
	RedactPatterns   []string
	Concurrency      int // 0 or <0 => auto

	// When true, removes extra blank lines between sections and files.
	Compact bool
}

type FileEntry struct {
	RelPath  string
	Size     int64
	IsBinary bool
	Content  []byte // omitted when skipped
}

type Report struct {
	FilesIncluded int
	FilesSkipped  int
	TotalBytes    int64
	Warnings      []string
}

func (c *Config) normalizedConcurrency() int {
	if c.Concurrency <= 0 {
		return runtime.NumCPU()
	}
	return c.Concurrency
}
