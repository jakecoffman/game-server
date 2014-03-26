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

func NewGame(r render.Render, db *gorp.DbMap, session sessions.Session, gameService GameService, log *log.Logger) {
	session.Clear()

	u, err := uuid.NewV4()
	if err != nil {
		log.Printf("UUID fail: %#v\n", err)
		r.JSON(500, map[string]string{"message": "can't generate UUID for some reason"})
		return
	}
	game := &Game{Id: u.String(), State: "lobby"}
	err = db.Insert(game)
	if err != nil {
		log.Printf("Insert fail: %#v", err)
		r.JSON(500, map[string]string{"message": "Failed to create game"})
		return
	}
	log.Println("New game started, UUID is " + u.String())

	// TODO: Require logins to create a new game, for now every new game gives you new player
	player := &Player{
		Game: game.Id,
		Role: Host,
	}
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

	gameService.Set(game.Id, &GameRelation{
		Players: []*Player{},
		Comm:    make(chan map[string]interface{}),
	})

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
		player = &Player{
			Game:     game.Id,
			ThisTurn: -1,
		}

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

func wsHandler(r render.Render, w http.ResponseWriter, req *http.Request, params martini.Params, db *gorp.DbMap, session sessions.Session, gameService GameService, log *log.Logger) {
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
	player.comm = make(chan map[string]interface{})

	// TODO: Thread safety
	// Check if game exists
	gameRelation, ok := gameService.Get(gameId)
	if !ok {
		log.Printf("No such game")
		return
	}
	// Add them to the game in progress
	gameRelation.Players = append(gameRelation.Players, player)
	defer playerDisconnect(gameRelation, gameId, *player)
	gameService.Set(gameId, gameRelation)

	// get the game from the db to load the state, other info
	g, err := db.Get(Game{}, gameId)
	if err != nil {
		log.Printf("Unable to find game %v", gameId)
		return
	}
	game := g.(*Game)

	// get the other players that are in the game?
	var players []Player
	_, err = db.Select(&players, "select * from players where game=?", gameId)
	if err != nil {
		log.Printf("Unable to find players in game: %#v", err)
		return
	}

	// start a goroutine dedicated to listening to the websocket
	wsReadChan := make(chan map[string]interface{})
	go func() {
		msg := map[string]interface{}{}
		for {
			// Blocks
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Error message from websocket: %#v", err)
				playerDisconnect(gameRelation, gameId, *player)
				gameRelation.Comm <- map[string]interface{}{"type": "leave"}
				return
			}
			log.Printf("Got message: %v", msg)
			wsReadChan <- msg
		}
	}()

	if player.Role == Host {
		log.Printf("Host is connected: %#v", player)
		player.conn.WriteJSON(map[string]interface{}{
			"type":    "players",
			"players": gameRelation.Players,
		})
		player.conn.WriteJSON(map[string]interface{}{
			"type":  "state",
			"state": game.State,
		})
		for {
			select {
			case msg := <-wsReadChan: // host website action
				switch {
				case msg["type"] == "state":
					log.Printf("Got state change request from host: %v", msg["state"])
					// TODO: check to make sure this is a valid state
					game.State = msg["state"].(string)
					if game.State == "start" {
						err = game.setBoard([]int{0, 0, 0, 0, 0, 0, 0, 0, 0})
						if err != nil {
							log.Printf("Unable to set game board: %#v", err)
							return
						}
					}
					count, err := db.Update(game)
					if err != nil || count == 0 {
						log.Printf("Unable to change game state: %v", err)
						return
					}
					log.Printf("Sending state %v to all players", msg["state"])
					board, err := game.getBoard()
					if err != nil {
						log.Printf("Error getting board: %#v", board)
					}
					for i, p := range gameRelation.Players {
						if p.Role == Host { // this will hang everything
							continue
						}
						log.Printf("Trying to sent to player %#v", p.Id)
						gameRelation.Players[i].comm <- map[string]interface{}{
							"type":  "update",
							"board": board,
							"state": "start",
						}
						log.Printf("Message sent to player %#v", p.Id)
					}
					player.conn.WriteJSON(map[string]interface{}{
						"type":  "update",
						"board": board,
						"state": "start",
					})
				default:
					log.Printf("Unknown web message from host: %#v", msg)
				}
			case msg := <-gameRelation.Comm: // server side message from player to host
				switch {
				case msg["type"] == "join":
					fallthrough
				case msg["type"] == "leave":
					log.Printf("player %v", msg["type"])
					// send a fresh list of players to the UI
					player.conn.WriteJSON(map[string]interface{}{
						"type":    "players",
						"players": gameRelation.Players,
					})
				case msg["type"] == "move":
					resolveRound := true
					for _, p := range gameRelation.Players {
						if p.Role != Host && p.ThisTurn == -1 {
							resolveRound = false
						}
					}
					if !resolveRound {
						continue
					}
					// all players have set their moves, update the board and send it out
					thisRound := []int{0, 0, 0, 0, 0, 0, 0, 0, 0}
					for _, p := range gameRelation.Players {
						if p.Role != Host {
							if thisRound[p.ThisTurn] == 0 {
								thisRound[p.ThisTurn] = p.Id
							} else {
								thisRound[p.ThisTurn] = 0 // two players went in the same spot
							}
						}
					}
					board, err := game.getBoard()
					if err != nil {
						log.Printf("Error getting board: %#v", err)
						return
					}
					for i, v := range board {
						if v == 0 {
							board[i] = thisRound[i]
						}
					}

					game.setBoard(board)
					count, err := db.Update(game)
					if err != nil || count == 0 {
						log.Printf("Unable to save game after move: %v", err)
						return
					}
					for i, p := range gameRelation.Players {
						if p.Role == Host {
							continue
						}
						gameRelation.Players[i].ThisTurn = -1
						gameRelation.Players[i].comm <- map[string]interface{}{
							"type":  "update",
							"board": board,
							"state": "start",
						}
					}
					player.conn.WriteJSON(map[string]interface{}{
						"type":  "update",
						"board": board,
						"state": "start",
					})
				default:
					log.Printf("Unknown message from player: %#v", msg)
				}
			}
		}
	} else {
		log.Printf("Player is connected: %#v", player)
		board, _ := game.getBoard()
		// There may not be a board yet so just try and send it
		player.conn.WriteJSON(map[string]interface{}{
			"type":  "update",
			"state": game.State,
			"board": board,
		})
		// Tell the host we've joined
		gameRelation.Comm <- map[string]interface{}{"type": "join"}
		player.ThisTurn = -1
		log.Printf("Player %v waiting for messages", player.Id)
		for {
			select {
			case msg := <-wsReadChan: // player website action
				switch {
				case msg["type"] == "move":
					// the player move comes as an integer from [0-8] representing the location of the move
					// TODO: assert this is the case before saving
					player.ThisTurn = int(msg["move"].(float64))
					gameRelation.Comm <- msg
				default:
					log.Printf("Unknown web message from player: %#v", msg)
				}
			case msg := <-player.comm: // messages from host
				// it may be safe to just take any message from the host and just send it
				log.Printf("Sending %v to player %v", msg["type"], player.Id)
				player.conn.WriteJSON(msg)
			}
		}
	}
}

func playerDisconnect(game *GameRelation, gameId string, player Player) {
	// Since Games represents CONNECTED players, we need to delete them when they dc
	for i := range game.Players {
		if player.Id == game.Players[i].Id {
			game.Players = append(game.Players[:i], game.Players[i+1:]...)
			return
		}
	}
	log.Printf("Failed to remove player")
}
