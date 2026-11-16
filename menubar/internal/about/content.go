package about

import "github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"

const (
	Name        = "Landline"
	CreditsLine = "From 8th Light"
)

type Content struct {
	Name        string
	Version     string
	CreditsLine string
	Link        string
}

func ContentFor(build buildinfo.Build) Content {
	return Content{Name: Name, Version: build.Display(), CreditsLine: CreditsLine, Link: Repository}
}
