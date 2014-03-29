package main

import (
	"testing"
)

func Test_GameService(t *testing.T) {
	db := initDb("services_test.db")
	db.DropTables()
	db = initDb("services_test.db")

	gs := GameServiceImpl{ChannelMap: map[string]*Channels{}}
	game, player, err := gs.NewGame(db)
	if err != nil {
		t.Errorf("New game error: %#v", err)
		return
	}
	hostRead := gs.HostJoin(game.Id)
	defer gs.HostLeave(game.Id)
	if hostRead == nil {
		t.Errorf("Failed to initialize host channels")
		return
	}

	playerRead, hostWrite := gs.PlayerJoin(game.Id, player.Id)
	defer gs.PlayerLeave(game.Id, player.Id)

	if playerRead == nil || hostWrite == nil {
		t.Errorf("Failed to initialize player channels")
		return
	}

	expected := Message{"hi": "hello"}
	var actual Message
	go func() {
		actual = <-hostRead
	}()
	hostWrite <- expected

	if actual["hi"] != expected["hi"] {
		t.Errorf("Couldn't send from player to host")
		return
	}

	var actual2 Message
	go func() {
		actual2 = <-playerRead
	}()
	gs.Broadcast(game.Id, expected)

	if actual2["hi"] != expected["hi"] {
		t.Errorf("Couldn't send from host to player")
		return
	}
}
