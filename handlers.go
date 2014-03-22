package main

import (
	"log"

	"github.com/coopernurse/gorp"
	"github.com/martini-contrib/render"
	"github.com/nu7hatch/gouuid"
)

func NewGame(r render.Render, db *gorp.DbMap, log *log.Logger) {
	u, err := uuid.NewV4()
	if err != nil {
		log.Printf("UUID fail: %v\n", err)
		r.JSON(500, map[string]string{"message": "can't generate UUID for some reason"})
		return
	}
	game := &Game{Id: u.String(), Players: []Player{}}
	db.Insert(game)
	log.Println("New game started, UUID is " + u.String())
	r.JSON(200, map[string]string{"uuid": u.String()})
}
