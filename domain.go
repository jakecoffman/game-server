package main

type Role int

const (
	Mafia = iota
	Townsperson
)

type Player struct {
	Id   int    `json:"id"`
	Game string `json:"string"`
	Role Role   `json:"role"`
	Host bool   `json:"host"`
}

type Game struct {
	Id string `json:"id"` // UUID
}
