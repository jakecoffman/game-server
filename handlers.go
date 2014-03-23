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
	if session.Get("player") != player {
		log.Printf("WAT")
		return
	}

	Games[game.Id] = &GameRelation{
		Players: []*Player{},
		Comm:    make(chan map[string]interface{}),
	}

	r.JSON(200, map[string]string{"uuid": u.String()})
}

func GetGame(r render.Render, params martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	gameId := params["id"]
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
	} else {
		player = sPlayer.(*Player)
		// TODO: this would screw with any games they are currently already in
		player.Game = game.Id
		log.Printf("Returning player id is: %#v", player.Id)
	}
	session.Set("player", player)

	// write something to the connection to get this to save?
	log.Printf("Setting player's id to %#v", player.Id)

	// inform the UI of who this is
	if player.Role == Host {
		r.JSON(200, map[string]interface{}{"type": "host", "host": true})
	} else {
		r.JSON(200, map[string]interface{}{"type": "host", "host": false})
	}
}

func wsHandler(r render.Render, w http.ResponseWriter, req *http.Request, params martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	conn, err := websocket.Upgrade(w, req, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()
	log.Println("Succesfully upgraded connection")

	// get things ready
	gameId := params["id"]
	p := session.Get("player")
	if p == nil {
		log.Println("Player not found in session")
		return
	}
	player := p.(*Player)
	player.conn = conn

	// TODO: Thread safety
	if _, ok := Games[gameId]; !ok {
		log.Printf("No such game")
		return
	}
	Games[gameId].Players = append(Games[gameId].Players, player)
	defer playerDisconnect(Games, gameId, *player)

	// start a goroutine dedicated to listening to the websocket
	wsReadChan := make(chan map[string]interface{})
	go func() {
		var msg map[string]interface{}
		for {
			// Blocks
			err := conn.ReadJSON(msg)
			if err != nil {
				playerDisconnect(Games, gameId, *player)
				Games[gameId].Comm <- map[string]interface{}{"type": "leave"}
				return
			}
			log.Printf("%#v", msg)
			wsReadChan <- msg
		}
	}()

	if player.Role == Host {
		player.conn.WriteJSON(map[string]interface{}{
			"type":    "players",
			"players": Games[gameId].Players,
		})
		for {
			select {
			case msg := <-wsReadChan: // host website action
				println(msg)
			case msg := <-Games[gameId].Comm: // server side message from player to host
				switch {
				case msg["type"] == "join":
					fallthrough
				case msg["type"] == "leave":
					// send a fresh list of players to the UI
					player.conn.WriteJSON(map[string]interface{}{
						"type":    "players",
						"players": Games[gameId].Players,
					})
				}
			}
		}
	} else {
		// Tell the host we've joined
		Games[gameId].Comm <- map[string]interface{}{"type": "join"}
		for {
			select {
			case msg := <-wsReadChan: // player website action
				println(msg)
			}
		}
	}
}

func playerDisconnect(Games map[string]*GameRelation, gameId string, player Player) {
	// Since Games represents CONNECTED players, we need to delete them when they dc
	for i := range Games[gameId].Players {
		if player.Id == Games[gameId].Players[i].Id {
			Games[gameId].Players = append(Games[gameId].Players[:i], Games[gameId].Players[i+1:]...)
			return
		}
	}
	log.Printf("Failed to remove player")
}
