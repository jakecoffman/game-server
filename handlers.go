package main

import (
	"log"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	"github.com/nu7hatch/gouuid"
)

func NewGame(r render.Render, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	session.Clear()

	u, err := uuid.NewV4()
	if err != nil {
		log.Printf("UUID fail: %#v\n", err)
		r.JSON(500, map[string]string{"message": "can't generate UUID for some reason"})
		return
	}
	game := &Game{Id: u.String()}
	err = db.Insert(game)
	if err != nil {
		log.Printf("Insert fail: %#v", err)
		r.JSON(500, map[string]string{"message": "Failed to create game"})
		return
	}
	log.Println("New game started, UUID is " + u.String())

	// TODO: Require logins to create a new game, for now every new game gives you new player
	player := &Player{Game: game.Id, Role: Host}
	err = db.Insert(player)
	if err != nil {
		log.Printf("New player could not be inserted")
		return
	}

	session.Set("player", player)

	r.JSON(200, map[string]string{"uuid": u.String()})
}

func GetGame(r render.Render, p martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	gameId := p["id"]
	obj, err := db.Get(Game{}, gameId)
	if err != nil {
		log.Printf("Error querying DB: %#v", err)
		return
	}
	if obj == nil {
		log.Printf("No such game: %#v", gameId)
		return
	}
	game := obj.(*Game)

	// see if player is rejoining
	sPlayer := session.Get("player")
	var player *Player
	if sPlayer == nil {
		// no, it's a new player
		player = &Player{Game: game.Id}

		// save to db so we can find them if they disconnect
		err = db.Insert(player)
		if err != nil {
			log.Printf("New player could not be inserted")
			return
		}
		session.Set("player", player)
	} else {
		player = sPlayer.(*Player)
		// are they a part of this game?
		if player.Game != game.Id {
			// TODO: Allow this. Is player joining this game? Are they just watching?
			log.Printf("Player tried to join game %#v: %#v", game.Id, player)
			return
		}
		log.Printf("Returning player id is: %#v", player.Id)
	}

	// write something to the connection to get this to save?
	log.Printf("Setting player's id to %#v", player.Id)

	// inform the UI of who this is
	if player.Role == Host {
		r.JSON(200, map[string]interface{}{"type": "host", "host": true})
	} else {
		r.JSON(200, map[string]interface{}{"type": "host", "host": false})
	}
}

type gameRelation struct {
	Players []Player
}

var gameMap map[string]gameRelation

func wsHandler(r render.Render, w http.ResponseWriter, req *http.Request, p martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	conn, err := websocket.Upgrade(w, req, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()
	gameId := p["id"]
	connections[gameId] = conn
	defer delete(connections, gameId)

	log.Println("Succesfully upgraded connection")

	for {
		// Blocks
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		log.Println(string(msg))
	}
}
