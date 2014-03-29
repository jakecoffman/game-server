package main

import (
	"log"

	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
)

func PlayerConn(player *Player, game *Game, gameService GameService, ws *websocket.Conn, wsReadChan chan Message, db *gorp.DbMap) {
	log.Printf("Player is connected: %#v", player.Id)
	playerRead, hostWrite := gameService.PlayerJoin(game.Id, player.Id)
	defer gameService.PlayerLeave(game.Id, player.Id)
	board, _ := game.getBoard()
	// There may not be a board yet so just try and send it
	ws.WriteJSON(Message{
		"type":  "update",
		"state": game.State,
		"board": board,
	})

	hostWrite <- Message{"type": "join"}

	player.ThisTurn = -1
	log.Printf("Player %v waiting for messages", player.Id)
	for {
		select {
		case msg, ok := <-wsReadChan: // player website action
			switch {
			case msg["type"] == "move":
				log.Printf("Sending move to host")
				// the player move comes as an integer from [0-8] representing the location of the move
				// TODO: assert this is the case before saving
				player.ThisTurn = int(msg["move"].(float64))
				count, err := db.Update(player)
				if count == 0 || err != nil {
					log.Printf("Failed to update moving player: %#v", player)
					return
				}

				hostWrite <- msg
			default:
				if !ok {
					return
				} else {
					log.Printf("Unknown web message from player: %#v", msg)
				}
			}
		case msg := <-playerRead: // messages from host
			// it may be safe to just take any message from the host and just send it
			log.Printf("Sending %v to player %v", msg["type"], player.Id)
			ws.WriteJSON(msg)
		}
	}
}

func HostConn(player *Player, game *Game, gameService GameService, ws *websocket.Conn, wsReadChan chan Message, db *gorp.DbMap) {
	log.Printf("Host is connected: %#v", player.Id)
	hostRead := gameService.HostJoin(game.Id)
	defer gameService.HostLeave(game.Id)

	// get the other players that are in the game
	var players []*Player
	_, err := db.Select(&players, "select * from players where game=?", game.Id)
	if err != nil {
		log.Printf("Unable to find players in game: %#v", err)
		return
	}
	ws.WriteJSON(Message{
		"type":    "players",
		"players": players,
	})
	ws.WriteJSON(Message{
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
				board, err := game.getBoard()
				if err != nil {
					log.Printf("Error getting board: %#v", board)
				}
				log.Printf("Sending state %v to all players", msg["state"])
				gameService.Broadcast(game.Id, Message{
					"type":  "update",
					"board": board,
					"state": "start",
				})
				log.Printf("Updating UI")
				ws.WriteJSON(Message{
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
		case msg := <-hostRead: // server side message from player to host
			switch {
			case msg["type"] == "join":
				fallthrough
			case msg["type"] == "leave":
				log.Printf("player %v", msg["type"])
				// send a fresh list of players to the UI
				players = nil
				_, err = db.Select(&players, "select * from players where game=?", game.Id)
				if err != nil {
					log.Printf("Failed to select players for game %v", game.Id)
					return
				}
				ws.WriteJSON(Message{
					"type":    "players",
					"players": players,
				})
			case msg["type"] == "move":
				log.Printf("Checking player move")
				players = nil
				_, err = db.Select(&players, "select * from players where game=?", game.Id)
				if err != nil {
					log.Printf("Failed to select players during move for game %v", game.Id)
					return
				}

				resolveRound := true
				for _, p := range players {
					if p.Role != Host && p.ThisTurn == -1 {
						resolveRound = false
					}
				}
				if !resolveRound {
					log.Printf("Round cannot be resolved")
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
					log.Printf("Resetting player %v turn", p.Id)
					count, err := db.Update(p)
					if count == 0 || err != nil {
						log.Printf("Failed to update playing player: %#v", p)
						return
					}
				}
				gameService.Broadcast(game.Id, Message{
					"type":  "update",
					"board": board,
					"state": "start",
				})
				ws.WriteJSON(Message{
					"type":  "update",
					"board": board,
					"state": "start",
				})
			default:
				log.Printf("Unknown message from player: %#v", msg)
			}
		}
	}
}
