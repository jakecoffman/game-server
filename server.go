package main

import (
	"database/sql"
	"encoding/gob"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// so we can save these to the session, save database queries
	gob.Register(&Player{})

	gameService := GameService{GameMap: map[string]*GameRelation{}}

	m := martini.Classic()

	store := sessions.NewCookieStore([]byte("secret123"))
	// store.Options(sessions.Options{HttpOnly: false})
	m.Use(sessions.Sessions("mafia", store))
	m.Use(render.Renderer())

	m.Post("/game", NewGame)
	m.Get("/game/:id", GetGame)
	m.Get("/ws/:id", wsHandler)

	m.Map(initDb("dev.db"))
	m.Map(gameService)

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
