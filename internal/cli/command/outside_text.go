package command

import (
	"strings"
	"unicode"
)

const (
	keepNewlinesAndTabs = "\n\t"
	keepNewlines        = "\n"
	keepNoBreaks        = ""
)

func printable(text string) string { return cleanOutsideText(text, keepNewlinesAndTabs) }

func printableLines(text string) string { return cleanOutsideText(text, keepNewlines) }

func printableLine(text string) string { return cleanOutsideText(text, keepNoBreaks) }

func cleanOutsideText(text, kept string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(kept, r) || r == 0x200c || r == 0x200d || r == 0xad:
			return r
		case unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf):
			return ' '
		}
		return r
	}, text)
}

type cleanedText struct{ answered error }

func (e cleanedText) Error() string { return printableLine(e.answered.Error()) }

func (e cleanedText) Unwrap() error { return e.answered }

func cleanedError(err error) error {
	if err == nil {
		return nil
	}
	return cleanedText{err}
}
