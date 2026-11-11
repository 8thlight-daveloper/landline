package inbox

import "github.com/8thlight-daveloper/landline/internal/model"

type Inbox interface {
	Select(log []model.Envelope, agent string, cursor int64, threads []string, limit int) model.Page
}
