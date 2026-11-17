package icon

import (
	_ "embed"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
)

var (
	//go:embed jack-template-44.png
	Template []byte
	//go:embed jack-dimmed-44.png
	Dimmed []byte
	//go:embed jack-about-440.png
	About []byte
	//go:embed state-connected-32.png
	Connected []byte
	//go:embed state-disconnected-32.png
	Disconnected []byte
)

func ForState(state string) []byte {
	switch state {
	case agentsview.Connected:
		return Connected
	case agentsview.Disconnected:
		return Disconnected
	}
	return nil
}
