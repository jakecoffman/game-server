package main

type Role int

const (
	Mafia = iota
	Townsperson
)

type Player struct {
	Id   int  `json:"id"`
	Role Role `json:"role"`
}

type Game struct {
	Id      string   `json:"id"` // UUID
	Players []Player `json:"players"`
}
