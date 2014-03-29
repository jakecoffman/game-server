package main

type Role int

const (
	Unassigned = iota
	Host
	Kibitz
)

type Player struct {
	Id   int    `json:"id"`     // maintain map of session ids to player ids so players may rejoin when dropped
	Game string `json:"string"` // current game we are in (foreign key)
	Role Role   `json:"role"`
}

type Game struct {
	Id    string `json:"id"` // UUID
	State string `json:"state"`
}

type Message map[string]interface{}
