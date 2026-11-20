package uuid

import (
	"github.com/rs/xid"
)

type generator struct{}

func NewGenerator() *generator { return &generator{} } //nolint: revive // Only one creation point. For prevent repo corruption

func (g *generator) New() string {
	id := xid.New()
	return id.String()
}
