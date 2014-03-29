package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/coopernurse/gorp"
	"github.com/nu7hatch/gouuid"
)

type GameService interface {
	NewGame(db *gorp.DbMap) (*Game, *Player, error)
	ConnectToGame(db *gorp.DbMap, gameId string, playerObj interface{}) (*Game, *Player, error)
	GetGame(db *gorp.DbMap, gameId string, playerId int) (*Game, *Player, error)
	HostJoin(gameId string) chan Message
	HostLeave(gameId string)
	PlayerJoin(gameId string, playerId int) (chan Message, chan Message)
	PlayerLeave(gameId string, playerId int)
	Broadcast(gameId string, msg Message)
}

type Channels struct {
	players map[int]chan Message
	host    chan Message
}

// TODO: this all needs to be in a different package
type GameServiceImpl struct {
	ChannelMap map[string]*Channels
}

func (gs *GameServiceImpl) HostJoin(gameId string) chan Message {
	gs.ChannelMap[gameId].host = make(chan Message)
	fmt.Printf("HOST IS INITIALIZED: %#v -- %#v\n", gameId, gs.ChannelMap[gameId].host)
	return gs.ChannelMap[gameId].host
}

func (gs *GameServiceImpl) HostLeave(gameId string) {
	close(gs.ChannelMap[gameId].host)
}

func (gs *GameServiceImpl) PlayerJoin(gameId string, playerId int) (chan Message, chan Message) {
	gs.ChannelMap[gameId].players[playerId] = make(chan Message)
	fmt.Printf("HOSTY HOSTY: %#v -- %#v\n", gameId, gs.ChannelMap[gameId].host)
	return gs.ChannelMap[gameId].players[playerId], gs.ChannelMap[gameId].host
}

func (gs *GameServiceImpl) PlayerLeave(gameId string, playerId int) {
	close(gs.ChannelMap[gameId].players[playerId])
	delete(gs.ChannelMap[gameId].players, playerId)
}

func (gs *GameServiceImpl) Broadcast(gameId string, msg Message) {
	for _, p := range gs.ChannelMap[gameId].players {
		p <- msg
	}
}

func (gs *GameServiceImpl) NewGame(db *gorp.DbMap) (*Game, *Player, error) {
	u, err := uuid.NewV4()
	if err != nil {
		return nil, nil, err
	}
	game := &Game{Id: u.String(), State: "lobby"}

	err = db.Insert(game)
	if err != nil {
		return nil, nil, err
	}

	player := NewPlayer(game.Id, Host)
	err = db.Insert(player)
	if err != nil {
		return nil, nil, err
	}

	gs.ChannelMap[game.Id] = &Channels{players: map[int]chan Message{}}

	return game, player, nil
}

func (gs *GameServiceImpl) ConnectToGame(db *gorp.DbMap, gameId string, playerObj interface{}) (*Game, *Player, error) {
	obj, err := db.Get(Game{}, gameId)
	if err != nil {
		return nil, nil, err
	}
	if obj == nil {
		return nil, nil, errors.New("Player not saved to session")
	}
	game := obj.(*Game)

	var player *Player
	if playerObj == nil { // no, it's a new player
		player = &Player{
			Game:     game.Id,
			ThisTurn: -1,
		}

		// save to db so we can find them if they disconnect
		err = db.Insert(player)
		if err != nil {
			return nil, nil, err
		}
	} else { // player is rejoining
		playerObj, err := db.Get(Player{}, playerObj)
		if err != nil {
			return nil, nil, err
		}
		player = playerObj.(*Player)
		// TODO: this would screw with any games they are currently already in?
		if player.Game != game.Id {
			player.Game = game.Id
			player.ThisTurn = -1
			count, err := db.Update(player)
			if count == 0 {
				return nil, nil, errors.New("Player update effected 0 rows")
			}
			if err != nil {
				return nil, nil, err
			}
			log.Printf("Joining player id is: %#v", player.Id)
		} else {
			log.Printf("Returning player id is: %#v", player.Id)
		}
	}

	return game, player, nil
}

func (gs *GameServiceImpl) GetGame(db *gorp.DbMap, gameId string, playerId int) (*Game, *Player, error) {
	obj, err := db.Get(Player{}, playerId)
	if err != nil {
		return nil, nil, err
	}
	player := obj.(*Player)

	// get the game from the db to load the state, other info
	g, err := db.Get(Game{}, gameId)
	if err != nil {
		return nil, nil, err
	}
	game := g.(*Game)
	return game, player, nil
}
