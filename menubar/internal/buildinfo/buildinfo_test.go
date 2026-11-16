package buildinfo

import "testing"

func TestShortKeepsSevenCharactersAndTheDirtyTag(t *testing.T) {
	cases := map[string]string{
		"":                                      "",
		"abc":                                   "abc",
		"a1b2c3d":                               "a1b2c3d",
		"a1b2c3d4e5f60718293a4b5c6d7e8f90":      "a1b2c3d",
		"a1b2c3d4e5f60718293a4b5c6d7e8f9-dirty": "a1b2c3d-dirty",
		"a1b2c3d-dirty":                         "a1b2c3d-dirty",
		"ünïcödé-long-value":                    "ünïcödé",
	}
	for in, want := range cases {
		if got := Short(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestLabelAndDisplayShowTheCommitOnlyWhenThereIsOne(t *testing.T) {
	stamped := Build{Version: "0.2.0", Commit: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}
	if got := stamped.Label("Menubar"); got != "Menubar 0.2.0 (a1b2c3d)" {
		t.Error(got)
	}
	if got := stamped.Display(); got != "0.2.0 (a1b2c3d)" {
		t.Error(got)
	}
	plain := Build{Version: "dev"}
	if plain.Label("Switchboard") != "Switchboard dev" || plain.Display() != "dev" {
		t.Error(plain.Label("Switchboard"), plain.Display())
	}
}

func TestStampedNamesAnEmptyVersionDev(t *testing.T) {
	if got := Stamped("", "").Version; got != "dev" {
		t.Error(got)
	}
	if got := Stamped("0.2.0", "x"); got != (Build{Version: "0.2.0", Commit: "x"}) {
		t.Error(got)
	}
}

func TestCleanTurnsControlCharactersIntoSpaces(t *testing.T) {
	got := Clean(Build{Version: "0.2.0\x1b[2J\nX", Commit: "a\tb"})
	if got.Version != "0.2.0 [2J X" || got.Commit != "a b" {
		t.Errorf("%+v", got)
	}
}
