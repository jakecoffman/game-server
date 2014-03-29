package main

import (
	"log"

	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
)

type HostAction func(Message, string, int, GameService, *websocket.Conn, *gorp.DbMap, *log.Logger) error
type PlayerAction func(Message, string, int, GameService, *websocket.Conn, chan Message, *gorp.DbMap, *log.Logger) error

// To define a game, all you need is to insert key-value pairs of message types to actions (handlers), and
// provide PlayerInit and HostInit functions.
var HostFromWeb map[string]HostAction = map[string]HostAction{}
var HostFromPlayer map[string]HostAction = map[string]HostAction{}
var PlayerFromWeb map[string]PlayerAction = map[string]PlayerAction{}
var PlayerFromHost map[string]PlayerAction = map[string]PlayerAction{}

func init() {
	PlayerFromWeb["move"] = playerMove
	PlayerFromHost["update"] = playerForward
	HostFromWeb["state"] = hostState
	HostFromPlayer["join"] = hostJoinLeave
	HostFromPlayer["leave"] = hostJoinLeave
	HostFromPlayer["move"] = hostMove
}

func PlayerInit(playerId int, gameId string, gameService GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap) error {
	log.Printf("Player is connected: %#v", playerId)

	game, player, err := gameService.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Couldn't get player and/or game")
		return err
	}

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

	_, err = db.Update(player)
	if err != nil {
		log.Printf("Failed to init player: %#v", err)
		return err
	}
	return nil
}

func HostInit(playerId int, gameId string, gameService GameService, ws *websocket.Conn, wsReadChan chan Message, db *gorp.DbMap) error {
	// get the other players that are in the game
	var players []*Player
	_, err := db.Select(&players, "select * from players where game=?", gameId)
	if err != nil {
		log.Printf("Unable to find players in game: %#v", err)
		return err
	}

	game, _, err := gameService.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Host failed to get game: %#v", err)
		return err
	}

	if game.State == "lobby" {
		ws.WriteJSON(Message{
			"type":    "players",
			"players": players,
		})
		ws.WriteJSON(Message{
			"type":  "state",
			"state": game.State,
		})
	} else {
		board, err := game.getBoard()
		if err != nil {
			log.Printf("Can't init with board: %#v", err)
			return err
		}
		ws.WriteJSON(Message{
			"type":  "update",
			"board": board,
			"state": "start",
		})
	}
	return nil
}

func playerMove(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Sending move to host")
	_, player, err := gs.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Could not get game: %#v", err)
		return err
	}

	// the player move comes as an integer from [0-8] representing the location of the move
	// TODO: assert this is the case before saving
	player.ThisTurn = int(msg["move"].(float64))
	_, err = db.Update(player)
	if err != nil {
		log.Printf("Failed to update moving player: %#v", player)
		return err
	}

	// send notice to the host that we've moved so it can attempt to resolve the current round
	hostWrite <- msg
	return nil
}

func playerForward(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, hostChan chan Message, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Sending %v to player %v", msg["type"], playerId)
	ws.WriteJSON(msg)
	return nil
}

func hostState(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Got state change request from host: %v", msg["state"])

	game, _, err := gs.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("%#v", err)
		return err
	}

	// TODO: check to make sure this is a valid state
	game.State = msg["state"].(string)
	if game.State == "start" {
		err = game.setBoard([]int{0, 0, 0, 0, 0, 0, 0, 0, 0})
		if err != nil {
			log.Printf("Unable to set game board: %#v", err)
			return err
		}
	}
	count, err := db.Update(game)
	if err != nil || count == 0 {
		log.Printf("Unable to change game state: %v", err)
		return err
	}
	board, err := game.getBoard()
	if err != nil {
		log.Printf("Error getting board: %#v", board)
		return err
	}
	log.Printf("Sending state %v to all players", msg["state"])
	gs.Broadcast(gameId, Message{
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
	return nil
}

func hostJoinLeave(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("player %v", msg["type"])
	// send a fresh list of players to the UI
	var players []*Player
	_, err := db.Select(&players, "select * from players where game=?", gameId)
	if err != nil {
		log.Printf("Failed to select players for game %v", gameId)
		return err
	}
	ws.WriteJSON(Message{
		"type":    "players",
		"players": players,
	})
	return nil
}

func hostMove(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Checking player move")
	game, _, err := gs.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Unable to get game in hostMove: %#", err)
		return err
	}

	var players []*Player
	_, err = db.Select(&players, "select * from players where game=?", gameId)
	if err != nil {
		log.Printf("Failed to select players during move for game %v", gameId)
		return err
	}

	resolveRound := true
	for _, p := range players {
		if p.Role != Host && p.ThisTurn == -1 {
			resolveRound = false
		}
	}
	if !resolveRound {
		log.Printf("Round cannot be resolved")
		return nil // not an error, just nothing to do
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
		return err
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
		return err
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
			return err
		}
	}
	gs.Broadcast(gameId, Message{
		"type":  "update",
		"board": board,
		"state": "start",
	})
	ws.WriteJSON(Message{
		"type":  "update",
		"board": board,
		"state": "start",
	})
	return nil
}
