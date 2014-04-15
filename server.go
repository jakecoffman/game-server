package main

import (
	"database/sql"
	"fmt"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	m := martini.Classic()

	store := sessions.NewCookieStore([]byte("secret123"))
	// store.Options(sessions.Options{HttpOnly: false})
	m.Use(sessions.Sessions("games", store))
	m.Use(render.Renderer())

	m.Get("/debug", DebugHandler)
	m.Get("/tictactoe", TicTacToeHandler)
	m.Post("/new/:game", NewGameHandler)
	m.Get("/game/:id", GetGameHandler)
	m.Get("/ws/:id", WebsocketHandler)

	m.Map(initDb("dev.db"))
	fmt.Printf("Creating game service")
	m.MapTo(&GameServiceImpl{ChannelMap: map[string]*Channels{}}, (*GameService)(nil))

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
