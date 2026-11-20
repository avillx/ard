package uuid

import (
	"github.com/rs/xid"
)

type generator struct{}

func NewGenerator() *generator { return &generator{} } //nolint: revive

func (g *generator) New() string {
	id := xid.New()
	return id.String()
}
