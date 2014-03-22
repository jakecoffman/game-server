package main

import (
	"log"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/coopernurse/gorp"
	"github.com/gorilla/websocket"
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
	game := &Game{Id: u.String()}
	err = db.Insert(game)
	if err != nil {
		log.Printf("Insert fail: %v", err)
		r.JSON(500, map[string]string{"message": "Failed to create game"})
		return
	}
	log.Println("New game started, UUID is " + u.String())
	r.JSON(200, map[string]string{"uuid": u.String()})
}

var connections map[string]*websocket.Conn

func wsHandler(r render.Render, w http.ResponseWriter, req *http.Request, p martini.Params, db *gorp.DbMap, log *log.Logger) {
	conn, err := websocket.Upgrade(w, req, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()
	log.Println("Succesfully upgraded connection")

	// get game from db
	obj, err := db.Get(Game{}, p["id"])
	if err != nil {
		conn.WriteJSON(map[string]string{"error": "error qurying db"})
		return
	}
	if obj == nil {
		conn.WriteJSON(map[string]string{"message": "no such game " + p["id"]})
		return
	}

	connections[p["id"]] = conn

	for {
		// Blocks
		_, msg, err := conn.ReadMessage()
		if err != nil {
			delete(connections, p["id"])
			return
		}
		log.Println(string(msg))

	}
}
