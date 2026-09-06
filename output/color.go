package output

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// Terminal colouring.
//
// Colour is enabled only when stdout is a real terminal, so piping to a file,
// grep, or a CI log never embeds escape sequences. NO_COLOR is honoured (see
// no-color.org), and SetColor lets --color=always|never override the
// detection.

var colorEnabled = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// SetColor forces colour on or off, for --color=always|never.
func SetColor(on bool) { colorEnabled = on }

// ColorEnabled reports the current setting.
func ColorEnabled() bool { return colorEnabled }

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
)

func wrap(code, s string) string {
	if !colorEnabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

func Green(s string) string  { return wrap(ansiGreen, s) }
func Red(s string) string    { return wrap(ansiRed, s) }
func Yellow(s string) string { return wrap(ansiYellow, s) }
func Blue(s string) string   { return wrap(ansiBlue, s) }
func Cyan(s string) string   { return wrap(ansiCyan, s) }
func Bold(s string) string   { return wrap(ansiBold, s) }
func Dim(s string) string    { return wrap(ansiDim, s) }

// ansiRe matches SGR escape sequences so widths can be measured on the
// characters a reader actually sees.
var ansiRe = regexp.MustCompile(`\033\[[0-9;]*m`)

// VisibleLen is the rune length of s ignoring colour escapes. Column widths
// must use this: measuring the raw string counts every escape byte and pushes
// later columns out of alignment.
func VisibleLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// Pad right-pads s to width visible columns.
func Pad(s string, width int) string {
	if n := VisibleLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
