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
	file, err := os.Open("public/tictactoe/index.html")
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

	// get the player and game ids so the handers can get the game and player objects later
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
		log.Printf("Host (player %v) has connected", playerId)

		hostRead := gameService.HostJoin(gameId)
		defer gameService.HostLeave(gameId)

		log.Printf("Initializing host")
		HostInit(playerId, gameId, gameService, ws, wsReadChan, db)

		for {
			select {
			case msg, ok := <-wsReadChan: // player website action
				if !ok {
					return
				}
				handled, err := dispatchMessage(HostFromWeb, msg, gameId, playerId, gameService, ws, nil, db, log)
				if err != nil {
					log.Printf("Error while handling message from web to host: %#v", err)
					return
				}
				if !handled {
					log.Printf("Unknown message from web to host: %#v", msg)
				}
			case msg := <-hostRead: // messages from host
				handled, err := dispatchMessage(HostFromPlayer, msg, gameId, playerId, gameService, ws, nil, db, log)
				if err != nil {
					log.Printf("Error while handling message from player to host: %#v", err)
					return
				}
				if !handled {
					log.Printf("Unknown message from player to host: %#v", msg)
				}
			}
		}
	} else {
		log.Printf("Player %v connected", playerId)

		playerRead, hostWrite := gameService.PlayerJoin(gameId, playerId)
		defer gameService.PlayerLeave(gameId, playerId)

		PlayerInit(playerId, gameId, gameService, ws, hostWrite, db)

		for {
			select {
			case msg, ok := <-wsReadChan: // player website action
				if !ok {
					return
				}
				handled, err := dispatchMessage(PlayerFromWeb, msg, gameId, playerId, gameService, ws, hostWrite, db, log)
				if err != nil {
					log.Printf("Error while handling message from web to player: %#v", err)
					return
				}
				if !handled {
					log.Printf("Unknown message from web to player: %#v", msg)
				}
			case msg := <-playerRead: // server side message from player to host
				handled, err := dispatchMessage(PlayerFromHost, msg, gameId, playerId, gameService, ws, hostWrite, db, log)
				if err != nil {
					log.Printf("Error while handling message from host to player: %#v", err)
					return
				}
				if !handled {
					log.Printf("Unknown message from host to player: %#v", msg)
				}
			}
		}
	}
}

func dispatchMessage(handleMap map[string]Action, msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap, log *log.Logger) (bool, error) {
	handled := false
	for msgType, action := range handleMap {
		if msgType == msg["type"] {
			err := action(msg, gameId, playerId, gs, ws, hostWrite, db, log)
			if err != nil {
				return false, err
			}
			handled = true
			break
		}
	}
	return handled, nil
}
