package main

import (
	"log"
	"os"
	"testing"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
)

var renderer = &MockRenderer{}
var session = &MockSession{}
var db = &gorp.DbMap{} // DB should not be used in these handler tests

func setUp() {
	session.Clear()
}

func Test_NewGameHandler(t *testing.T) {
	setUp()
	log := log.New(os.Stderr, "TEST: ", log.Flags())
	gameService := &MockGameService{
		Game:   &Game{Id: "Hello"},
		Player: &Player{Id: 1},
	}
	NewGameHandler(renderer, db, session, gameService, log)

	response := renderer.data.(Message)
	if renderer.status != 200 || response["uuid"] != "Hello" {
		t.Errorf("Failed renderer: %#v", renderer)
		return
	}

	if 1 != session.Get("player_id") {
		t.Errorf("Player ID not saved in session")
		return
	}
}

func Test_GetGameHandler_Host(t *testing.T) {
	setUp()
	log := log.New(os.Stderr, "TEST: ", log.Flags())
	gameService := &MockGameService{
		Game:   &Game{Id: "Hello"},
		Player: &Player{Id: 1, Role: Host},
	}
	params := martini.Params{"id": "asdf"}
	session.Set("player_id", 1)
	GetGameHandler(renderer, params, db, gameService, session, log)
	response := renderer.data.(Message)
	if renderer.status != 200 || response["type"] != "host" || response["host"] != true {
		t.Errorf("Failed to get proper response: %#v", response)
		return
	}
	if session.Get("player_id") != 1 {
		t.Errorf("Didn't put player ID in session", session.Get("player_id"))
		return
	}
}

func Test_GetGameHandler_Player(t *testing.T) {
	log := log.New(os.Stderr, "TEST: ", log.Flags())
	gameService := &MockGameService{
		Game:   &Game{Id: "Hello"},
		Player: &Player{Id: 7},
	}
	params := martini.Params{"id": "asdf"}
	GetGameHandler(renderer, params, db, gameService, session, log)
	response := renderer.data.(Message)
	if renderer.status != 200 || response["type"] != "host" || response["host"] != false {
		t.Errorf("Failed to get proper response: %#v", response)
		return
	}
	if session.Get("player_id") != 7 {
		t.Errorf("Didn't put player ID in session", session.Get("player_id"))
		return
	}
}

func Test_GameService_NewGame(t *testing.T) {
	// objs, err := db.Select(Game{}, "select * from games")
	// if objs == nil || err != nil || len(objs) != 1 {
	// 	t.Errorf("Failed to find game in DB: %#v, %#v", err, objs)
	// 	return
	// }
	// game := objs[0].(*Game)

	// objs, err = db.Select(Player{}, "select * from players where game=?", game.Id)
	// if objs == nil || len(objs) != 1 || err != nil {
	// 	t.Errorf("Failed to find host in new game: %#v, %#v", err, objs)
	// 	return
	// }
	// player := objs[0].(*Player)
}
