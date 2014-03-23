package main

import "github.com/gorilla/websocket"

type Role int

const (
	Unassigned = iota
	Host
	Mafia
	Townsperson
)

type Player struct {
	Id   int             `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game string          `json:"string"` // fk
	Role Role            `json:"role"`
	Conn *websocket.Conn `db:"-" json:"-"`
}

type Game struct {
	Id      string   `json:"id"` // UUID
	Players []Player `db:"-"`
}
