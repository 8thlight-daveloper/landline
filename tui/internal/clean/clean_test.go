package clean

import "testing"

func TestTextReplacesControlFormatAndSeparatorRunesAndInvalidBytes(t *testing.T) {
	cases := map[string]string{
		"a\x1b[2Jb": "a [2Jb", "a\u0085b": "a b", "a‮b": "a b", "a\nb": "a b", "a b": "a b",
		"a\x9bb": "a�b", "plain é 日本": "plain é 日本",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
