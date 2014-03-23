package main

import "github.com/gorilla/websocket"

type Role int

const (
	Unassigned = iota
	Host
	Mafia
	Townsperson
)

// These top structures are saved to the database

type Player struct {
	Id   int             `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game string          `json:"string"` // fk
	Role Role            `json:"role"`
	conn *websocket.Conn `db:"-" json:"-"` // unexported so gob doesn't try to seriealize it
}

type Game struct {
	Id      string   `json:"id"` // UUID
	Players []Player `db:"-"`
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
