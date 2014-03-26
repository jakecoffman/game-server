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
	Id   int    `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game string `json:"string"` // fk
	Role Role   `json:"role"`
	// connection variables
	conn *websocket.Conn             `db:"-" json:"-"` // unexported so gob doesn't try to seriealize it
	comm chan map[string]interface{} `db:"-" json:"-"`
	// game specific
	ThisTurn int `db:"this_turn"`
}

type Game struct {
	Id    string `json:"id"` // UUID
	State string `json:"state"`
	Board string `json:"board"` // JSON string of board (so it can be persisted in the DB)
}

// These structures are not currently saved to the database and represent games in progress

// A single games relations
type GameRelation struct {
	Players []*Player
	Comm    chan map[string]interface{} // allows players to send direct messages to the host
}

// Manages access and changes to games in progress
type GameState struct {
	// this is not thread safe so need to use locks at some point
	m map[string]*GameRelation
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

// TODO: Locking, interface this
type GameService struct {
	GameMap map[string]*GameRelation
}

func (games *GameService) Set(key string, value *GameRelation) {
	games.GameMap[key] = value
}

func (games *GameService) Get(key string) (*GameRelation, bool) {
	v, b := games.GameMap[key]
	return v, b
}
