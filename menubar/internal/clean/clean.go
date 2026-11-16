package clean

import (
	"strings"
	"unicode"
)

func Text(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, text)
}
