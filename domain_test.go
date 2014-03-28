package main

import (
	"testing"
)

func Test_GameService(t *testing.T) {
	gameId := "6"
	playerId := 4
	gs := GameServiceImpl{}
	gs.Register(gameId)
	hostRead := gs.HostJoin(gameId)
	defer gs.HostLeave(gameId)
	if hostRead == nil {
		t.Errorf("Failed to initialize host channels")
		return
	}

	playerRead, hostWrite := gs.PlayerJoin(gameId, playerId)
	defer gs.PlayerLeave(gameId, playerId)

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
	gs.Broadcast(gameId, expected)

	if actual2["hi"] != expected["hi"] {
		t.Errorf("Couldn't send from host to player")
		return
	}
}
