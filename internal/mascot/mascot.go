package mascot

import (
	"os"
	"strings"
)

var sprite = []string{
	"           ",
	"           ",
	"   #####   ",
	"  #######  ",
	" ## ### ## ",
	"### ### ###",
	"###########",
	" ######### ",
	"    ###    ",
	"   #####   ",
	"   # # #   ",
	"           ",
}

var palette = map[byte][3]int{
	'#': {67, 181, 230}, // logo blue
}

const reset = "\x1b[0m"

func Render(color bool) string {
	var sb strings.Builder
	for y := 0; y < len(sprite); y += 2 {
		top := sprite[y]
		bot := ""
		if y+1 < len(sprite) {
			bot = sprite[y+1]
		}
		// Width comes from the wider row: a short top row must not clip the bottom.
		w := max(len(top), len(bot))
		for x := 0; x < w; x++ {
			tc, tok := pixel(top, x)
			bc, bok := pixel(bot, x)
			sb.WriteString(cell(tc, tok, bc, bok, color))
		}
		if color {
			sb.WriteString(reset)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func Beside(right string, gap int, color bool) string {
	skull := strings.Split(strings.TrimRight(Render(color), "\n"), "\n")
	text := strings.Split(strings.Trim(right, "\n"), "\n")
	offset := (len(skull) - len(text)) / 2

	var sb strings.Builder
	pad := strings.Repeat(" ", gap)
	for i, s := range skull {
		sb.WriteString(s)
		if j := i - offset; j >= 0 && j < len(text) {
			sb.WriteString(pad)
			sb.WriteString(text[j])
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func pixel(row string, x int) ([3]int, bool) {
	if x >= len(row) {
		return [3]int{}, false
	}
	rgb, ok := palette[row[x]]
	return rgb, ok
}

func cell(top [3]int, topOK bool, bot [3]int, botOK bool, color bool) string {
	if !color {
		switch {
		case topOK && botOK:
			return "█"
		case topOK:
			return "▀"
		case botOK:
			return "▄"
		default:
			return " "
		}
	}
	switch {
	case topOK && botOK:
		return fg(top) + bg(bot) + "▀"
	case topOK:
		return reset + fg(top) + "▀"
	case botOK:
		return reset + fg(bot) + "▄"
	default:
		return reset + " "
	}
}

func fg(c [3]int) string { return "\x1b[38;2;" + rgb(c) + "m" }
func bg(c [3]int) string { return "\x1b[48;2;" + rgb(c) + "m" }

func rgb(c [3]int) string {
	return itoa(c[0]) + ";" + itoa(c[1]) + ";" + itoa(c[2])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [3]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func Colorable(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
