package main

import (
	"database/sql"
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"

	"github.com/coopernurse/gorp"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	m := martini.Classic()

	m.Use(render.Renderer())

	m.Post("/game", NewGame)

	m.Map(initDb("dev.db"))

	m.Run()
}

func initDb(name string) *gorp.DbMap {
	db, err := sql.Open("sqlite3", name)
	nilOrPanic(err)

	dbmap := &gorp.DbMap{Db: db, Dialect: gorp.SqliteDialect{}}

	err = dbmap.DropTables()
	nilOrPanic(err)
	dbmap.AddTableWithName(Game{}, "games").SetKeys(false, "Id")
	dbmap.AddTableWithName(Player{}, "players").SetKeys(true, "Id")

	// TODO: Use DB migration tool
	err = dbmap.CreateTablesIfNotExists()
	nilOrPanic(err)

	return dbmap
}

func nilOrPanic(err error) {
	if err != nil {
		panic(err)
	}
}
