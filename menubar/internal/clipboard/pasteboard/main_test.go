package pasteboard

import (
	"testing"

	"github.com/8thlight-daveloper/landline/testsupport/process/reaper"
)

func TestMain(m *testing.M) { reaper.Main(m, "landline-menubar-internal-clipboard-pasteboard-test-") }
