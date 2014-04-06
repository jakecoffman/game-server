package main

import (
	"encoding/json"
	"log"

	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
)

// tictactoe domain objects
type TicTacToe_Turn struct {
	Id     int
	Player int    // foreign key to player
	Game   string // foreign key to game (do we need this?)
	Move   int    // the last move the player entered
}

type TicTacToe_Board struct {
	Id    int
	Game  string // foreign key to game
	Board string // the board represented as a string
}

func (g TicTacToe_Board) getBoard() ([]int, error) {
	d := []int{}
	err := json.Unmarshal([]byte(g.Board), &d)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (g *TicTacToe_Board) setBoard(v []int) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	g.Board = string(b)
	return nil
}

type Action func(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap, log *log.Logger) error

// To define a game, all you need is to insert key-value pairs of message types to actions (handlers), and
// provide PlayerInit and HostInit functions.
var HostFromWeb map[string]Action
var HostFromPlayer map[string]Action
var PlayerFromWeb map[string]Action
var PlayerFromHost map[string]Action

func init() {
	PlayerFromWeb = map[string]Action{
		"move": playerMove,
	}
	PlayerFromHost = map[string]Action{
		"update": playerForward,
	}
	HostFromWeb = map[string]Action{
		"state": hostState,
	}
	HostFromPlayer = map[string]Action{
		"join":  hostJoinLeave,
		"leave": hostJoinLeave,
		"move":  hostMove,
	}
}

func PlayerInit(playerId int, gameId string, gameService GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap) error {
	log.Printf("Player is connected: %#v", playerId)

	game, _, err := gameService.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Couldn't get player and/or game")
		return err
	}

	if game.State == "start" {
		log.Printf("Player %#v rejoining game in play", playerId)
		board, err := getBoard(gameId, db)
		if err != nil {
			log.Printf("Can't get TTT board: %#v", err)
			return err
		}

		log.Printf("Got board, getting nicer one: %#v", board)
		niceBoard, err := board.getBoard()
		if err != nil {
			log.Printf("Unable to get nice board: %#v", err)
			return err
		}
		log.Printf("Got board for player %#v: %#v", playerId, board)

		// There may not be a board yet so just try and send it
		ws.WriteJSON(Message{
			"type":  "update",
			"state": game.State,
			"board": niceBoard,
		})
	} else {
		ws.WriteJSON(Message{
			"type":  "update",
			"state": game.State,
			"board": nil,
		})
	}

	// check to make sure this player has a turn row
	turn := TicTacToe_Turn{}
	err = db.SelectOne(&turn, "select * from tictactoe_turn where game=? and player=?", gameId, playerId)
	if err != nil {
		turn.Game = gameId
		turn.Player = playerId
		turn.Move = -1
		err = db.Insert(&turn)
		if err != nil {
			log.Printf("Unable to insert initial turn row: %#v", err)
			return err
		}
	}

	hostWrite <- Message{"type": "join"}
	return nil
}

// Called first when a host connects.
// NOTE that this may be called multiple times as a host may drop and reconnect.
func HostInit(playerId int, gameId string, gameService GameService, ws *websocket.Conn, wsReadChan chan Message, db *gorp.DbMap) error {
	log.Printf("Host initing")

	// since host is always the first to connect, setup tables if they don't already exist
	db.AddTableWithName(TicTacToe_Board{}, "tictactoe_board").SetKeys(true, "Id")
	db.AddTableWithName(TicTacToe_Turn{}, "tictactoe_turn").SetKeys(true, "Id")
	err := db.CreateTablesIfNotExists()
	if err != nil {
		log.Printf("Unable to create TTT tables: %#v", err)
		return err
	}

	log.Printf("Tables created")

	// get the game so we know what state we should be in
	game, _, err := gameService.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("Host failed to get game: %#v", err)
		return err
	}

	log.Printf("got game")

	if game.State == "lobby" {
		log.Printf("Game is still in lobby")
		// get the other players that are in the game so we can update the lobby
		var players []*Player
		_, err = db.Select(&players, "select * from players where game=?", gameId)
		if err != nil {
			log.Printf("Unable to find players in game: %#v", err)
			return err
		}
		log.Printf("Found %v players in game", len(players))

		ws.WriteJSON(Message{
			"type":    "players",
			"players": players,
		})
		ws.WriteJSON(Message{
			"type":  "state",
			"state": game.State,
		})
	} else {
		log.Printf("Host rejoining game in progress")
		// get the game board so we can send an update
		board, err := getBoard(gameId, db)
		if err != nil {
			log.Printf("Could not get board, this might not be an error: %#v", err)
			return err
		}

		log.Printf("Getting nicer board: %#v", board)
		niceBoard, err := board.getBoard()
		if err != nil {
			log.Printf("Can't init with board: %#v", err)
			return err
		}
		ws.WriteJSON(Message{
			"type":  "update",
			"board": niceBoard,
			"state": "start",
		})
	}
	return nil
}

func playerMove(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, hostWrite chan Message, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Sending move to host")
	turn := TicTacToe_Turn{}
	err := db.SelectOne(&turn, "select * from tictactoe_turn where game=? and player=?", gameId, playerId)
	if err != nil {
		log.Printf("Unable to get turn in move: %#v", err)
		return err
	}

	// the player move comes as an integer from [0-8] representing the location of the move
	// TODO: assert this is the case before saving
	turn.Move = int(msg["move"].(float64))
	_, err = db.Update(&turn)
	if err != nil {
		log.Printf("Failed to update moving player: %#v", err)
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

func hostState(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, _ chan Message, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Got state change request from host: %v", msg["state"])

	game, _, err := gs.GetGame(db, gameId, playerId)
	if err != nil {
		log.Printf("%#v", err)
		return err
	}

	// TODO: check to make sure this is a valid state
	game.State = msg["state"].(string)
	var board *TicTacToe_Board
	if game.State == "start" {
		board = &TicTacToe_Board{Game: gameId}
		log.Printf("Setting up starting objects")
		// we are starting a game, so insert a new board
		err = board.setBoard([]int{0, 0, 0, 0, 0, 0, 0, 0, 0})
		if err != nil {
			log.Printf("Unable to set game board: %#v", err)
			return err
		}
		log.Printf("Inserting board: %#v", board)
		err = db.Insert(board)
		if err != nil {
			log.Printf("Couldn't insert board: %#v", err)
			return err
		}
	} else {
		board, err = getBoard(gameId, db)
		if err != nil {
			log.Printf("Unable to get board: %#v", err)
			return err
		}
	}
	log.Printf("Updating game state to %#v", game.State)
	count, err := db.Update(game)
	if err != nil || count == 0 {
		log.Printf("Unable to change game state: %v", err)
		return err
	}
	niceBoard, err := board.getBoard()
	if err != nil {
		log.Printf("Error getting board: %#v", board)
		return err
	}
	log.Printf("Sending state %v to all players", msg["state"])
	gs.Broadcast(gameId, Message{
		"type":  "update",
		"board": niceBoard,
		"state": "start",
	})
	log.Printf("Updating UI")
	ws.WriteJSON(Message{
		"type":  "update",
		"board": niceBoard,
		"state": "start",
	})
	return nil
}

func hostJoinLeave(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, _ chan Message, db *gorp.DbMap, log *log.Logger) error {
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

func hostMove(msg Message, gameId string, playerId int, gs GameService, ws *websocket.Conn, _ chan Message, db *gorp.DbMap, log *log.Logger) error {
	log.Printf("Checking player move")

	var players []*Player
	_, err := db.Select(&players, "select * from players where game=?", gameId)
	if err != nil {
		log.Printf("Failed to select players during move for game %v", gameId)
		return err
	}

	resolveRound := true
	for _, p := range players {
		if p.Role == Host {
			continue
		}
		turn := TicTacToe_Turn{}
		err = db.SelectOne(&turn, "select * from tictactoe_turn where game=? and player=?", gameId, p.Id)
		if err != nil {
			log.Printf("Couldn't get turn %#v", err)
			return err
		}

		if turn.Move == -1 {
			resolveRound = false
		}
	}
	if !resolveRound {
		log.Printf("Round cannot be resolved")
		return nil // not an error, just nothing to do
	}

	board, err := getBoard(gameId, db)
	if err != nil {
		log.Printf("Couldn't get board: %#v", err)
		return err
	}

	// all players have set their moves, update the board
	thisRound := []int{0, 0, 0, 0, 0, 0, 0, 0, 0}
	for _, p := range players {
		if p.Role == Host {
			continue
		}
		turn := TicTacToe_Turn{}
		err = db.SelectOne(&turn, "select * from tictactoe_turn where game=? and player=?", gameId, p.Id)
		if err != nil {
			log.Printf("Couldn't get turn %#v", err)
			return err
		}
		if thisRound[turn.Move] == 0 {
			thisRound[turn.Move] = p.Id
		} else {
			thisRound[turn.Move] = 0 // two players went in the same spot
		}
	}
	niceBoard, err := board.getBoard()
	if err != nil {
		log.Printf("Error getting board: %#v", err)
		return err
	}
	for i, v := range niceBoard {
		if v == 0 {
			niceBoard[i] = thisRound[i]
		}
	}

	board.setBoard(niceBoard)
	count, err := db.Update(board)
	if err != nil || count == 0 {
		log.Printf("Unable to save board after move: %v", err)
		return err
	}
	for _, p := range players {
		if p.Role == Host {
			continue
		}
		turn := TicTacToe_Turn{}
		err = db.SelectOne(&turn, "select * from tictactoe_turn where game=? and player=?", gameId, p.Id)
		if err != nil {
			log.Printf("Couldn't get turn %#v", err)
			return err
		}
		turn.Move = -1
		log.Printf("Resetting player %v turn", p.Id)
		count, err := db.Update(&turn)
		if count == 0 || err != nil {
			log.Printf("Failed to update player turn: %#v -- %#v", err, p.Id)
			return err
		}
	}
	gs.Broadcast(gameId, Message{
		"type":  "update",
		"board": niceBoard,
		"state": "start",
	})
	ws.WriteJSON(Message{
		"type":  "update",
		"board": niceBoard,
		"state": "start",
	})
	return nil
}

// helpers
func getBoard(gameId string, db *gorp.DbMap) (*TicTacToe_Board, error) {
	board := &TicTacToe_Board{}
	err := db.SelectOne(board, "select * from tictactoe_board where game=?", gameId)
	return board, err
}
