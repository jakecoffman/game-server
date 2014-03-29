package main

import "encoding/json"

type Role int

const (
	Unassigned = iota
	Host
	Kibitz
)

type Player struct {
	Id       int    `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game     string `json:"string"` // current game we are in (foreign key)
	Role     Role   `json:"role"`
	ThisTurn int    `json:"this_turn"`
}

type Game struct {
	Id    string `json:"id"` // UUID
	State string `json:"state"`
	Board string `json:"board"` // JSON string of board (so it can be persisted in the DB)
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

type Message map[string]interface{}
