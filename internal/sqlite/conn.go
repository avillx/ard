package sqlite

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

const (
	SQLDriver    = "sqlite3"
	DataBasePath = "./agent.db"
)

func NewDataBase() (*sql.DB, error) {
	return sql.Open(SQLDriver, DataBasePath)
}
