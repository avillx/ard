package uuid

import (
	"github.com/rs/xid"
)

type generator struct{}

func NewGenerator() *generator { return &generator{} }

func (g *generator) New() string {
	id := xid.New()
	return id.String()
}
