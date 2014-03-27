package main

import (
	"database/sql"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	_ "github.com/mattn/go-sqlite3"
)

type Channels struct {
	players map[int]chan Message
	host    chan Message
}

var ChannelMap map[string]*Channels

func main() {
	ChannelMap = map[string]*Channels{}

	m := martini.Classic()

	store := sessions.NewCookieStore([]byte("secret123"))
	// store.Options(sessions.Options{HttpOnly: false})
	m.Use(sessions.Sessions("games", store))
	m.Use(render.Renderer())

	m.Post("/game", NewGameHandler)
	m.Get("/game/:id", GetGameHandler)
	m.Get("/ws/:id", WebsocketHandler)

	m.Map(initDb("dev.db"))
	m.Map(&GameService{})

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
