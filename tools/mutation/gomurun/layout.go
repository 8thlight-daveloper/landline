package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

type layout struct {
	components  string
	contracts   string
	accepted    string
	gomuVersion string
}

func defaultLayout() layout {
	return layout{
		components:  "internal",
		contracts:   "contracts",
		accepted:    "tools/mutation/accepted",
		gomuVersion: "v0.3.0",
	}
}

func (place layout) normalized() (layout, error) {
	folders := []struct {
		flag  string
		value *string
	}{
		{"--components", &place.components},
		{"--contracts", &place.contracts},
		{"--accepted", &place.accepted},
	}
	for _, each := range folders {
		clean := path.Clean(filepath.ToSlash(*each.value))
		if !insideProject(clean) {
			return layout{}, fmt.Errorf("%s must be a folder inside the project, got %q", each.flag, *each.value)
		}
		*each.value = clean
	}
	return place, nil
}

func insideProject(clean string) bool {
	return !path.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, "../")
}

func segments(clean string) int {
	if clean == "." {
		return 0
	}
	return len(strings.Split(clean, "/"))
}
