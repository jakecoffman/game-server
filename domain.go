package main

import (
	"encoding/json"

	"github.com/gorilla/websocket"
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
	Id       int                         `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game     string                      `json:"string"` // current game we are in (foreign key)
	Role     Role                        `json:"role"`
	conn     *websocket.Conn             `db:"-" json:"-"` // unexported so gob doesn't try to seriealize it
	comm     chan map[string]interface{} `db:"-" json:"-"`
	ThisTurn int                         `db:"this_turn"`
}

type Game struct {
	Id       string                      `json:"id"` // UUID
	State    string                      `json:"state"`
	Board    string                      `json:"board"` // JSON string of board (so it can be persisted in the DB)
	hostChan chan map[string]interface{} `json:"-" db:"-"`
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
