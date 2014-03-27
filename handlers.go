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
func NewGameHandler(r render.Render, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	game, err := NewGame()
	if err != nil {
		log.Printf("New game failed: %#v\n", err)
		r.JSON(500, Message{"message": "Failed to create a new game"})
		return
	}

	err = db.Insert(game)
	if err != nil {
		log.Printf("Insert fail: %#v", err)
		r.JSON(500, Message{"message": "Failed to create game"})
		return
	}
	log.Println("New game started, UUID is " + game.Id)

	// TODO: require logins for hosts

	player := NewPlayer(game.Id, Host)
	err = db.Insert(player)
	if err != nil {
		log.Printf("New player could not be inserted: %#v", err)
		r.JSON(500, Message{"message": "Failed to create player"})
		return
	}

	session.Set("player_id", player.Id)

	r.JSON(200, Message{"uuid": game.Id})
}

// this resource is hit first before a player can connect with websockets, partially due to the session not being able to be set
// on the websocket handler
func GetGameHandler(r render.Render, params martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
	// get the game from the DB
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

	// is the player is rejoining?
	obj = session.Get("player_id")
	var player *Player
	if obj == nil { // no, it's a new player
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
	} else { // player is rejoining
		obj, err := db.Get(Player{}, obj)
		if err != nil {
			log.Printf("Failed to get player from DB")
			return
		}
		player = obj.(*Player)
		// TODO: this would screw with any games they are currently already in?
		if player.Game != game.Id {
			player.Game = game.Id
			player.ThisTurn = -1
			count, err := db.Update(player)
			if count == 0 || err != nil {
				log.Printf("Failed to update rejoining player: %#v", player)
				return
			}
			log.Printf("Joining player id is: %#v", player.Id)
		} else {
			log.Printf("Returning player id is: %#v", player.Id)
		}
	}
	// save to the session so the websocket handler so we recognize them when they join a game
	session.Set("player_id", player.Id)

	// Create a Channel object so players and host can join
	ChannelMap[game.Id] = &Channels{players: map[int]chan Message{}}

	// inform the UI of who this is
	if player.Role == Host {
		r.JSON(200, Message{"type": "host", "host": true})
	} else {
		r.JSON(200, Message{"type": "host", "host": false})
	}
}

// handles the websocket connections for the game
func WebsocketHandler(r render.Render, w http.ResponseWriter, req *http.Request, params martini.Params, db *gorp.DbMap, session sessions.Session, log *log.Logger) {
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
	player.conn = conn
	player.comm = make(chan Message)
	defer close(player.comm)

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
			err := conn.ReadJSON(&msg)
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
		// create the host channel. when this channel is closed/nil it means the host is not connected
		ChannelMap[gameId].host = make(chan Message)
		defer close(ChannelMap[gameId].host)

		// get the other players that are in the game
		var players []Player
		_, err = db.Select(&players, "select * from players where game=?", gameId)
		if err != nil {
			log.Printf("Unable to find players in game: %#v", err)
			return
		}

		log.Printf("Host is connected: %#v", player)
		player.conn.WriteJSON(Message{
			"type":    "players",
			"players": players,
		})
		player.conn.WriteJSON(Message{
			"type":  "state",
			"state": game.State,
		})
		for {
			select {
			case msg, ok := <-wsReadChan: // host website action
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
					for _, p := range players {
						if p.Role == Host { // this will hang everything
							continue
						}
						log.Printf("Trying to sent to player %#v", p.Id)
						ChannelMap[gameId].players[p.Id] <- Message{
							"type":  "update",
							"board": board,
							"state": "start",
						}
						log.Printf("Message sent to player %#v", p.Id)
					}
					player.conn.WriteJSON(Message{
						"type":  "update",
						"board": board,
						"state": "start",
					})
				default:
					if !ok {
						// the connection was dropped so return
						return
					} else {
						log.Printf("Unknown web message from host: %#v", msg)
					}
				}
			case msg := <-ChannelMap[gameId].host: // server side message from player to host
				switch {
				case msg["type"] == "join":
					fallthrough
				case msg["type"] == "leave":
					log.Printf("player %v", msg["type"])
					// send a fresh list of players to the UI
					_, err = db.Select(&players, "select * from players where game=?", gameId)
					if err != nil {
						log.Printf("Failed to select players for game %v", gameId)
						return
					}

					var connected []*Player
					for _, p := range players {
						if _, ok := ChannelMap[gameId].players[p.Id]; ok {
							connected = append(connected, &p)
						}
					}

					player.conn.WriteJSON(Message{
						"type":    "players",
						"players": connected,
					})
				case msg["type"] == "move":
					_, err = db.Select(&players, "select * from players where game=?", gameId)
					if err != nil {
						log.Printf("Failed to select players during move for game %v", gameId)
						return
					}

					resolveRound := true
					for _, p := range players {
						if p.Role != Host && p.ThisTurn == -1 {
							resolveRound = false
						}
					}
					if !resolveRound {
						continue
					}

					// all players have set their moves, update the board and send it out
					thisRound := []int{0, 0, 0, 0, 0, 0, 0, 0, 0}
					for _, p := range players {
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
					for _, p := range players {
						if p.Role == Host {
							continue
						}
						p.ThisTurn = -1
						count, err := db.Update(player)
						if count == 0 || err != nil {
							log.Printf("Failed to update rejoining player: %#v", player)
							return
						}
						ChannelMap[gameId].players[p.Id] <- Message{
							"type":  "update",
							"board": board,
							"state": "start",
						}
					}
					player.conn.WriteJSON(Message{
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
		player.comm = make(chan Message)
		ChannelMap[gameId].players[player.Id] = player.comm
		log.Printf("Player is connected: %#v", player)
		board, _ := game.getBoard()
		// There may not be a board yet so just try and send it
		player.conn.WriteJSON(Message{
			"type":  "update",
			"state": game.State,
			"board": board,
		})
		// Tell the host we've joined
		ChannelMap[gameId].host <- Message{"type": "join"}
		player.ThisTurn = -1
		log.Printf("Player %v waiting for messages", player.Id)
		for {
			select {
			case msg, ok := <-wsReadChan: // player website action
				switch {
				case msg["type"] == "move":
					// the player move comes as an integer from [0-8] representing the location of the move
					// TODO: assert this is the case before saving
					player.ThisTurn = int(msg["move"].(float64))
					ChannelMap[gameId].host <- msg
				default:
					if !ok {
						return
					} else {
						log.Printf("Unknown web message from player: %#v", msg)
					}
				}
			case msg := <-player.comm: // messages from host
				// it may be safe to just take any message from the host and just send it
				log.Printf("Sending %v to player %v", msg["type"], player.Id)
				player.conn.WriteJSON(msg)
			}
		}
	}
}
