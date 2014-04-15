package main

type Role int

const (
	Unassigned = iota
	Host
	Kibitz
)

type Player struct {
	Id    int    `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Name  string `json:"name"`   // a name players may enter for themselves to more easily be identified
	Color string `json:"color"`  // a hex color to customize the player image
	Game  string `json:"string"` // current game we are in (foreign key)
	Role  Role   `json:"role"`
}

type Game struct {
	Id    string `json:"id"` // UUID
	State string `json:"state"`
	Type  string `json:"type"` // type of game (tictactoe, trivia, etc)
}

type Message map[string]interface{}
