package main

import (
	"log"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
)

// Creates a new game and player (the host)
func NewGameHandler(r render.Render, db *gorp.DbMap, session sessions.Session, gameService GameService, log *log.Logger) {
	game, player, err := gameService.NewGame(db)
	if err != nil {
		log.Printf("Failed to create game: %v", err)
		r.JSON(500, Message{"message": "Failed to create game"})
		return
	}
	log.Printf("New game started, UUID %v and host Id %v", game.Id, player.Id)

	// TODO: require logins for hosts

	session.Set("player_id", player.Id)

	r.JSON(200, Message{"uuid": game.Id})
}

// this resource is hit first before a player can connect with websockets, partially due to the session not being able to be set
// on the websocket handler
func GetGameHandler(r render.Render, params martini.Params, db *gorp.DbMap, gameService GameService, session sessions.Session, log *log.Logger) {
	// get the game from the DB
	gameId := params["id"]
	// and the player from the session
	obj := session.Get("player_id")

	_, player, err := gameService.ConnectToGame(db, gameId, obj)
	if err != nil {
		log.Printf("Failed to connect to game: %v", err)
		r.JSON(500, Message{"message": "Failed to connect to game"})
		return
	}

	// save to the session so the websocket handler so we recognize them when they join a game
	session.Set("player_id", player.Id)

	// inform the UI of who this is
	if player.Role == Host {
		r.JSON(200, Message{"type": "host", "host": true})
	} else {
		r.JSON(200, Message{"type": "host", "host": false})
	}
}

// handles the websocket connections for the game
func WebsocketHandler(r render.Render, w http.ResponseWriter, req *http.Request, params martini.Params, db *gorp.DbMap, gameService GameService, session sessions.Session, log *log.Logger) {
	// upgrade to websocket
	ws, err := websocket.Upgrade(w, req, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()
	log.Println("Succesfully upgraded connection")

	// get the player
	gameId := params["id"]
	p := session.Get("player_id")
	if p == nil {
		log.Println("Player not found in session")
		return
	}
	obj, err := db.Get(Player{}, p)
	if err != nil {
		log.Printf("Could not find player with id %v in database", p)
		return
	}
	player := obj.(*Player)

	// get the game from the db to load the state, other info
	g, err := db.Get(Game{}, gameId)
	if err != nil {
		log.Printf("Unable to find game %v", gameId)
		return
	}
	game := g.(*Game)

	// start a goroutine dedicated to listening to the websocket
	wsReadChan := make(chan Message)
	go func() {
		msg := Message{}
		for {
			// Blocks
			err := ws.ReadJSON(&msg)
			if err != nil {
				log.Printf("Error message from websocket: %#v", err)
				return
			}
			log.Printf("Got message: %v", msg)
			wsReadChan <- msg
		}
	}()
	defer func() {
		if _, ok := <-wsReadChan; !ok {
			close(wsReadChan)
		}
	}()

	if player.Role == Host {
		HostConn(player, game, gameService, ws, wsReadChan, db)
	} else {
		PlayerConn(player, game, gameService, ws, wsReadChan, db)
	}
}
