package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
)

func TicTacToeHandler() string {
	file, err := os.Open("public/tictactoe.html")
	if err != nil {
		return err.Error()
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

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

	// get the player and game from the database
	gameId := params["id"]
	p := session.Get("player_id")
	if p == nil {
		log.Println("Player not found in session")
		return
	}
	playerId := p.(int)

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

	_, player, err := gameService.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Unable to get game here: %#v", err)
		return
	}

	if player.Role == Host {
		hostRead := gameService.HostJoin(gameId)
		defer gameService.HostLeave(gameId)

		HostInit(playerId, gameId, gameService, ws, wsReadChan, db)

		for {
			select {
			case msg, ok := <-wsReadChan: // player website action
				if !ok {
					return
				}
				handled := false
				for msgType, action := range HostFromWeb {
					if msgType == msg["type"] {
						err = action(msg, gameId, playerId, gameService, ws, db, log)
						if err != nil {
							return
						}
						handled = true
						break
					}
				}
				if !handled {
					log.Printf("Unknown web message from player: %#v", msg)
				}
			case msg := <-hostRead: // messages from host
				handled := false
				for msgType, action := range HostFromPlayer {
					if msgType == msg["type"] {
						err = action(msg, gameId, playerId, gameService, ws, db, log)
						if err != nil {
							return
						}
						handled = true
						break
					}
				}
				if !handled {
					log.Printf("Unknown web message from player: %#v", msg)
				}
			}
		}
	} else {
		playerRead, hostWrite := gameService.PlayerJoin(gameId, playerId)
		defer gameService.PlayerLeave(gameId, playerId)

		PlayerInit(playerId, gameId, gameService, ws, hostWrite, db)

		for {
			select {
			case msg, ok := <-wsReadChan: // player website action
				if !ok {
					return
				}
				handled := false
				for msgType, action := range PlayerFromWeb {
					if msgType == msg["type"] {
						err = action(msg, gameId, playerId, gameService, ws, hostWrite, db, log)
						if err != nil {
							return
						}
						handled = true
						break
					}
				}
				if !handled {
					log.Printf("Unknown web message from player: %#v", msg)
				}
			case msg := <-playerRead: // server side message from player to host
				handled := false
				for msgType, action := range PlayerFromHost {
					if msgType == msg["type"] {
						err = action(msg, gameId, playerId, gameService, ws, hostWrite, db, log)
						if err != nil {
							return
						}
						handled = true
						break
					}
				}
				if !handled {
					log.Printf("Unknown message from host: %#v", msg)
				}
			}
		}
	}
}
