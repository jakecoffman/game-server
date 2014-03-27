package main

import (
	"encoding/json"

	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
	"github.com/nu7hatch/gouuid"
)

type Role int

const (
	Unassigned = iota
	Host
	Mafia
	Townsperson
)

// These top structures are saved to the database

type Player struct {
	Id       int             `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game     string          `json:"string"` // current game we are in (foreign key)
	Role     Role            `json:"role"`
	conn     *websocket.Conn `db:"-" json:"-"` // unexported so gob doesn't try to seriealize it
	comm     chan Message    `db:"-" json:"-"`
	ThisTurn int             `db:"this_turn"`
}

func NewPlayer(gameId string, role Role) *Player {
	return &Player{
		Game: gameId,
		Role: role,
	}
}

type Game struct {
	Id       string       `json:"id"` // UUID
	State    string       `json:"state"`
	Board    string       `json:"board"` // JSON string of board (so it can be persisted in the DB)
	hostChan chan Message `json:"-" db:"-"`
}

func (g Game) getBoard() ([]int, error) {
	d := []int{}
	err := json.Unmarshal([]byte(g.Board), &d)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (g *Game) setBoard(v []int) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	g.Board = string(b)
	return nil
}

// Not saved to database

// TODO: make this an interface. for now since we are all in the same namespace this is easier
type GameService struct {
}

func (gs *GameService) NewGame(db *gorp.DbMap) (*Game, *Player, error) {
	u, err := uuid.NewV4()
	if err != nil {
		return nil, nil, err
	}
	game := &Game{Id: u.String(), State: "lobby"}

	err = db.Insert(game)
	if err != nil {
		return nil, nil, err
	}

	player := NewPlayer(game.Id, Host)
	err = db.Insert(player)
	if err != nil {
		return nil, nil, err
	}

	return game, player, nil
}

type Message map[string]interface{}
