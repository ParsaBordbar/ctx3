package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/parsabordbar/ctx3/internal/tokens"
	toon "github.com/toon-format/toon-go"
)

// Shared plumbing for the command adapters. Every analysis command takes an
// optional [directory], renders one string, and either prints it or writes it
// to -o. Keeping that here is what lets a cmd file stay "parse flags, call the
// package, hand back a string".

var quietTokens bool

func fitBudget(output string, budget int) string {
	out, trunc := tokens.Fit(output, budget)
	if trunc {
		fmt.Fprintf(os.Stderr, "trimmed to fit %s tokens\n", tokens.Format(budget))
	}
	return out
}

// dirArg returns the optional positional path, defaulting to the working dir.
func dirArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}

// encodeStructured renders v as JSON or TOON. Callers switch on their own flags
// first, so reaching this with neither set is a programming error, not input.
func encodeStructured(v any, asTOON bool) (string, error) {
	if asTOON {
		b, err := toon.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("encoding TOON: %w", err)
		}
		return string(b), nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding JSON: %w", err)
	}
	return string(b), nil
}

// writeOut delivers rendered output: to stdout when path is empty or "-", to a
// file otherwise. label names the artifact in the note printed on stderr, which
// stays off stdout so `ctx3 x -o - | …` pipes cleanly.
func writeOut(output, path, label string) error {
	body := strings.TrimRight(output, "\n") + "\n"
	if path == "" || path == "-" {
		fmt.Print(body)
		if !quietTokens {
			fmt.Fprintln(os.Stderr, tokens.Line(body))
		}
		return nil
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "%s written to %s (%s)\n", label, path, tokens.Line(body))
	return nil
}
