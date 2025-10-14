package pack

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	doublestar "github.com/bmatcuk/doublestar/v4"
	ignore "github.com/sabhiram/go-gitignore"
)

type dirNode struct {
	Name     string
	Children []*dirNode
	Files    []string // relative file paths under this dir
}

type walkResult struct {
	rootTree *dirNode
	files    []FileEntry
	report   Report
}

// WalkAndCollect walks cfg.RootDir and returns files + a directory tree.
// Precedence: hard excludes (.git/node_modules) → includes (if any) → ignores/.gitignore
func WalkAndCollect(ctx context.Context, cfg Config) ([]FileEntry, *dirNode, Report, error) {
	if cfg.RootDir == "" {
		return nil, nil, Report{}, errors.New("empty RootDir")
	}
	rootAbs, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, nil, Report{}, err
	}
	// Load .gitignore at repo root
	var gitIg *ignore.GitIgnore
	if cfg.RespectGitignore {
		if gi := loadGitignore(rootAbs); gi != nil {
			gitIg = gi
		}
	}

	tree := &dirNode{Name: ".", Children: nil, Files: nil}
	byDir := map[string]*dirNode{".": tree}
	result := walkResult{rootTree: tree}

	candidates := []string{}
	err = filepath.WalkDir(rootAbs, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			result.report.Warnings = append(result.report.Warnings, werr.Error())
			return nil
		}
		rel, _ := filepath.Rel(rootAbs, p)
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if isHardExcludedDir(rel) {
				return fs.SkipDir
			}
			ensureDirNode(byDir, rel)
			return nil
		}

		// includes (highest precedence)
		included := true
		if len(cfg.IncludeGlobs) > 0 {
			included = anyGlobMatch(rel, cfg.IncludeGlobs)
		}
		if !included {
			return nil
		}

		// if not explicitly included, apply ignores
		if len(cfg.IncludeGlobs) == 0 {
			if gitIg != nil && gitIg.MatchesPath(rel) {
				return nil
			}
			if anyGlobMatch(rel, cfg.IgnoreGlobs) {
				return nil
			}
		}

		// per-file cap
		if cfg.MaxFileBytes > 0 {
			if info, ierr := os.Stat(p); ierr == nil && info.Size() > cfg.MaxFileBytes {
				result.report.FilesSkipped++
				return nil
			}
		}

		candidates = append(candidates, rel)
		parentDir := parent(rel)
		node := ensureDirNode(byDir, parentDir)
		node.Files = append(node.Files, rel)
		return nil
	})
	if err != nil {
		return nil, nil, result.report, err
	}

	// deterministic order
	if cfg.SortByExt {
		sort.Slice(candidates, func(i, j int) bool {
			exti := strings.ToLower(filepath.Ext(candidates[i]))
			extj := strings.ToLower(filepath.Ext(candidates[j]))
			if exti == extj {
				return candidates[i] < candidates[j]
			}
			return exti < extj
		})
	} else {
		sort.Strings(candidates)
	}

	// Read contents with total cap + binary handling
	concurrency := cfg.normalizedConcurrency()
	type readItem struct{ rel, pth string }
	items := make(chan readItem, len(candidates))
	for _, rel := range candidates {
		items <- readItem{rel: rel, pth: filepath.Join(rootAbs, filepath.FromSlash(rel))}
	}
	close(items)

	type outItem struct {
		entry         FileEntry
		err           error
		skipped       bool
		skippedReason string
		size          int64
	}

	out := make(chan outItem, len(candidates))
	workers := minInt(concurrency, maxInt(1, len(candidates)))
	for w := 0; w < workers; w++ {
		go func() {
			for it := range items {
				select {
				case <-ctx.Done():
					out <- outItem{err: ctx.Err()}
					return
				default:
				}
				entry, size, skipped, reason, rerr := readOne(it.rel, it.pth, cfg)
				out <- outItem{entry: entry, err: rerr, size: size, skipped: skipped, skippedReason: reason}
			}
		}()
	}

	var picked []FileEntry
	var total int64
	var firstErr error
	for i := 0; i < len(candidates); i++ {
		oi := <-out
		if oi.err != nil && firstErr == nil {
			firstErr = oi.err
		}
		if oi.skipped {
			result.report.FilesSkipped++
			if oi.skippedReason != "" {
				result.report.Warnings = append(result.report.Warnings, oi.skippedReason)
			}
			continue
		}
		if cfg.MaxTotalBytes > 0 && total+oi.size > cfg.MaxTotalBytes {
			result.report.FilesSkipped++
			result.report.Warnings = append(result.report.Warnings, "max total bytes exceeded; remaining files skipped")
			continue
		}
		picked = append(picked, oi.entry)
		total += oi.size
	}

	result.files = picked
	result.report.FilesIncluded = len(picked)
	result.report.TotalBytes = total
	return result.files, result.rootTree, result.report, firstErr
}

func readOne(rel, abs string, cfg Config) (FileEntry, int64, bool, string, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return FileEntry{}, 0, true, "stat error: " + rel, nil
	}
	size := info.Size()

	f, err := os.Open(abs)
	if err != nil {
		return FileEntry{}, 0, true, "open error: " + rel, nil
	}
	defer f.Close()

	var sniffCap int64 = 8192
	head := make([]byte, min64(size, sniffCap))
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	isBin := isBinary(head)

	var content []byte
	switch {
	case isBin && cfg.BinaryHandling == BinarySkip:
		return FileEntry{RelPath: rel, Size: size, IsBinary: true}, size, true, "binary skipped: " + rel, nil
	case isBin && cfg.BinaryHandling == BinaryHex:
		all, rerr := os.ReadFile(abs)
		if rerr != nil {
			return FileEntry{}, 0, true, "read error: " + rel, nil
		}
		dst := make([]byte, len(all)*2)
		hexEncode(dst, all)
		content = dst
	case isBin && cfg.BinaryHandling == BinaryBase64:
		all, rerr := os.ReadFile(abs)
		if rerr != nil {
			return FileEntry{}, 0, true, "read error: " + rel, nil
		}
		content = make([]byte, base64EncodedLen(len(all)))
		base64Encode(content, all)
	default:
		all, rerr := os.ReadFile(abs)
		if rerr != nil {
			return FileEntry{}, 0, true, "read error: " + rel, nil
		}
		content = all
	}

	if len(cfg.RedactPatterns) > 0 && len(content) > 0 {
		rc, _ := redactAll(content, cfg.RedactPatterns)
		content = rc
	}

	return FileEntry{
		RelPath:  rel,
		Size:     size,
		IsBinary: isBin,
		Content:  content,
	}, int64(len(content)), false, "", nil
}

func loadGitignore(rootAbs string) *ignore.GitIgnore {
	p := filepath.Join(rootAbs, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		if gi, err := ignore.CompileIgnoreFile(p); err == nil {
			return gi
		}
	}
	return nil
}

func isHardExcludedDir(rel string) bool {
	parts := strings.Split(rel, "/")
	last := parts[len(parts)-1]
	return last == ".git" || last == "node_modules"
}

func anyGlobMatch(rel string, globs []string) bool {
	name := filepath.FromSlash(rel)
	for _, g := range globs {
		pat := filepath.FromSlash(g)
		if ok, _ := doublestar.PathMatch(pat, name); ok {
			return true
		}
	}
	return false
}

func parent(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return "."
}

func ensureDirNode(index map[string]*dirNode, rel string) *dirNode {
	if rel == "" {
		rel = "."
	}
	if n, ok := index[rel]; ok {
		return n
	}
	par := parent(rel)
	parentNode := ensureDirNode(index, par)
	n := &dirNode{Name: rel, Children: nil, Files: nil}
	parentNode.Children = append(parentNode.Children, n)
	index[rel] = n
	return n
}

// --- helpers ---

func isBinary(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	if bytes.IndexByte(buf, 0x00) >= 0 {
		return true
	}
	if utf8.Valid(buf) {
		return false
	}
	nng := 0
	for _, b := range buf {
		if b < 0x09 || (b > 0x0D && b < 0x20) {
			nng++
		}
	}
	return float64(nng)/float64(len(buf)) > 0.3
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

const hexdigits = "0123456789abcdef"

func hexEncode(dst, src []byte) {
	for i, j := 0, 0; i < len(src); i, j = i+1, j+2 {
		b := src[i]
		dst[j] = hexdigits[b>>4]
		dst[j+1] = hexdigits[b&0x0f]
	}
}

const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func base64EncodedLen(n int) int { return (n + 2) / 3 * 4 }

func base64Encode(dst, src []byte) {
	di := 0
	for i := 0; i < len(src); i += 3 {
		var v uint
		remain := len(src) - i
		switch {
		case remain >= 3:
			v = uint(src[i])<<16 | uint(src[i+1])<<8 | uint(src[i+2])
			dst[di+0] = b64[(v>>18)&0x3F]
			dst[di+1] = b64[(v>>12)&0x3F]
			dst[di+2] = b64[(v>>6)&0x3F]
			dst[di+3] = b64[v&0x3F]
		case remain == 2:
			v = uint(src[i])<<10 | uint(src[i+1])<<2
			dst[di+0] = b64[(v>>12)&0x3F]
			dst[di+1] = b64[(v>>6)&0x3F]
			dst[di+2] = b64[v&0x3F]
			dst[di+3] = '='
		case remain == 1:
			v = uint(src[i]) << 4
			dst[di+0] = b64[(v>>6)&0x3F]
			dst[di+1] = b64[v&0x3F]
			dst[di+2] = '='
			dst[di+3] = '='
		}
		di += 4
	}
}

func redactAll(content []byte, patterns []string) ([]byte, []string) {
	s := string(content)
	var warns []string
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			warns = append(warns, "invalid redact regex: "+p)
			continue
		}
		s = re.ReplaceAllString(s, "***")
	}
	return []byte(s), warns
}
