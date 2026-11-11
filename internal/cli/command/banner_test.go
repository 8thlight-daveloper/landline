package command

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestThePlainBannerIsNineRowsOfEighteenCellsWithNoEscapes(t *testing.T) {
	t.Parallel()
	lines := strings.Split(strings.TrimSuffix(Banner(false), "\n"), "\n")
	if len(lines) != 9 {
		t.Fatalf("%d rows", len(lines))
	}
	for i, line := range lines {
		if utf8.RuneCountInString(line) != 18 {
			t.Errorf("row %d is %d cells: %q", i, utf8.RuneCountInString(line), line)
		}
	}
	if strings.ContainsRune(Banner(false), 0x1b) {
		t.Fatal("the plain banner holds an escape")
	}
	if lines[2] != "██████████████████" || lines[3] != "████    ██    ████" || lines[4] != "████  ▒▒██  ▒▒████" {
		t.Fatalf("body or eyes wrong:\n%q\n%q\n%q", lines[2], lines[3], lines[4])
	}
}

func TestTheColoredBannerUsesBrandedBlueAndWhiteEyesAndLeavesNoColorOpenAtTheEndOfARow(t *testing.T) {
	t.Parallel()
	out := Banner(true)
	for _, want := range []string{"\x1b[38;2;0;113;206m", "\x1b[38;2;255;255;255m", "\x1b[38;2;20;20;20m"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q", want)
		}
	}
	for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		last := strings.LastIndex(line, "\x1b[")
		if last >= 0 && !strings.HasPrefix(line[last:], "\x1b[0m") {
			t.Errorf("row %d leaves a color open: %q", i, line)
		}
	}
}

func TestTheBannerHasTheSameShapeColoredOrNot(t *testing.T) {
	t.Parallel()
	strip := func(text string) string {
		var out strings.Builder
		for i := 0; i < len(text); i++ {
			if text[i] == 0x1b {
				for i < len(text) && text[i] != 'm' {
					i++
				}
				continue
			}
			out.WriteByte(text[i])
		}
		return out.String()
	}
	plain, colored := strings.Split(Banner(false), "\n"), strings.Split(strip(Banner(true)), "\n")
	for i := range plain {
		if len([]rune(plain[i])) != len([]rune(colored[i])) {
			t.Errorf("row %d: %q vs %q", i, plain[i], colored[i])
		}
	}
}
