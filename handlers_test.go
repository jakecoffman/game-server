package main

import (
	"log"
	"os"
	"testing"

	"github.com/coopernurse/gorp"
)

var renderer = &MockRenderer{}
var session = &MockSession{}

func setUp() *gorp.DbMap {
	db := initDb("test.db")
	db.DropTables()
	db = initDb("test.db")

	session.Clear()

	return db
}

func Test_NewGameHandler(t *testing.T) {
	db := setUp()

	log := log.New(os.Stderr, "TEST: ", log.Flags())
	NewGameHandler(renderer, db, session, log)

	objs, err := db.Select(Game{}, "select * from games")
	if objs == nil || err != nil || len(objs) != 1 {
		t.Errorf("Failed to find game in DB: %#v, %#v", err, objs)
		return
	}
	game := objs[0].(*Game)

	objs, err = db.Select(Player{}, "select * from players where game=?", game.Id)
	if objs == nil || len(objs) != 1 || err != nil {
		t.Errorf("Failed to find host in new game: %#v, %#v", err, objs)
		return
	}
	player := objs[0].(*Player)

	if player.Id != session.Get("player_id") {
		t.Errorf("Player ID not saved in session")
		return
	}
	response := renderer.data.(map[string]interface{})
	if renderer.status != 200 || response["uuid"] != game.Id {
		t.Errorf("Failed renderer: %#v", renderer)
		return
	}
}
