package style

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/parsabordbar/ctx3/internal/mascot"
)

type Palette struct{ on bool }

var (
	Plain = Palette{}
	ANSI  = Palette{on: true}
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	accent  = "\x1b[38;2;67;181;230m"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func Auto(f *os.File) Palette {
	if mascot.Colorable(f) {
		return ANSI
	}
	return Plain
}

func Resolve(mode string, f *os.File) (Palette, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return Auto(f), nil
	case "always", "on", "yes":
		return ANSI, nil
	case "never", "off", "no":
		return Plain, nil
	}
	return Plain, fmt.Errorf("--color must be auto, always or never, got %q", mode)
}

func (p Palette) Enabled() bool { return p.on }

func (p Palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return code + s + reset
}

func (p Palette) Bold(s string) string    { return p.wrap(bold, s) }
func (p Palette) Dim(s string) string     { return p.wrap(dim, s) }
func (p Palette) Title(s string) string   { return p.wrap(bold+accent, s) }
func (p Palette) Accent(s string) string  { return p.wrap(accent, s) }
func (p Palette) Name(s string) string    { return p.wrap(bold+cyan, s) }
func (p Palette) Path(s string) string    { return p.wrap(cyan, s) }
func (p Palette) Keyword(s string) string { return p.wrap(magenta, s) }
func (p Palette) Ok(s string) string      { return p.wrap(green, s) }
func (p Palette) Warn(s string) string    { return p.wrap(yellow, s) }
func (p Palette) Err(s string) string     { return p.wrap(red, s) }

func Strip(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	return ansiRe.ReplaceAllString(s, "")
}

func Width(s string) int { return len([]rune(Strip(s))) }

func Ljust(s string, width int) string {
	if pad := width - Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
