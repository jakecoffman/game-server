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

func NewGame(r render.Render, db *gorp.DbMap, log *log.Logger) {
	u, err := uuid.NewV4()
	if err != nil {
		log.Printf("UUID fail: %v\n", err)
		r.JSON(500, map[string]string{"message": "can't generate UUID for some reason"})
		return
	}
	game := &Game{Id: u.String()}
	err = db.Insert(game)
	if err != nil {
		log.Printf("Insert fail: %v", err)
		r.JSON(500, map[string]string{"message": "Failed to create game"})
		return
	}
	log.Println("New game started, UUID is " + u.String())
	r.JSON(200, map[string]string{"uuid": u.String()})
}

func GetGame(r render.Render, p martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	gameId := p["id"]
	obj, err := db.Get(Game{}, gameId)
	if err != nil {
		log.Printf("Error querying DB: %v", err)
		return
	}
	if obj == nil {
		log.Printf("No such game: %v", gameId)
		return
	}
	game := obj.(*Game)

	var player *Player

	// see if player is rejoining
	id := session.Get("player")
	log.Printf("Returning player id is: %#v", id)

	if id == nil {
		// no, it's a new player
		player = &Player{Game: game.Id}
	} else {
		// get player from DB and see if they are a part of this game
		obj, err = db.Get(Player{}, id)
		if err != nil {
			log.Printf("Failed getting player: %v", err)
			return
		}
		if obj == nil {
			// player's session ID is screwed up or the server lost everything: new player
			player = &Player{Game: game.Id}
		} else {
			player = obj.(*Player)
			if player.Game != game.Id {
				// TODO: Allow this.
				log.Printf("Player tried to join a new game: %v", player)
				return
			}
		}
	}

	// check if there are any other players in the game already, if not this is the host
	count, err := db.SelectInt("select count(*) from players where game=?", game.Id)
	if err != nil {
		log.Printf("Failed to check if other players in game: %v", err)
		return
	}
	if count == 0 {
		player.Role = Host
	}

	// save to db so we can find them if they disconnect
	if player.Id == 0 {
		err = db.Insert(player)
		if err != nil {
			log.Printf("New player could not be inserted")
			return
		}
	}
	session.Set("player", player.Id)
	// write something to the connection to get this to save?
	log.Printf("Setting player's id to %#v", player.Id)

	if player.Role == Host {
		r.JSON(200, map[string]interface{}{"type": "host", "host": true})
	} else {
		r.JSON(200, map[string]interface{}{"type": "host", "host": false})
	}
}

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
	conn.WriteJSON(map[string]string{"hi": "yo"})

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
