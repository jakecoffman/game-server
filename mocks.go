package main

import (
	"html/template"

	"github.com/coopernurse/gorp"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
)

type MockRenderer struct {
	status int
	data   interface{}
}

func (m *MockRenderer) JSON(status int, v interface{}) {
	m.status = status
	m.data = v
}
func (m *MockRenderer) HTML(status int, name string, v interface{}, htmlOpt ...render.HTMLOptions) {
	return
}
func (m *MockRenderer) Error(status int) {
	return
}
func (m *MockRenderer) Redirect(location string, status ...int) {
	return
}
func (m *MockRenderer) Template() *template.Template {
	return nil
}

type MockSession struct {
	data map[interface{}]interface{}
}

func (m *MockSession) Get(key interface{}) interface{} {
	v, _ := m.data[key]
	return v
}

func (m *MockSession) Set(key interface{}, val interface{}) {
	m.data[key] = val
}

func (m *MockSession) Delete(key interface{}) {
	delete(m.data, key)
}

func (m *MockSession) Clear() {
	m.data = map[interface{}]interface{}{}
}

func (m *MockSession) AddFlash(value interface{}, vars ...string) {

}

func (m *MockSession) Flashes(vars ...string) []interface{} {
	return nil
}

func (m *MockSession) Options(sessions.Options) {

}

type MockGameService struct {
	Game   *Game
	Player *Player
	Error  error
}

func (m *MockGameService) NewGame(db *gorp.DbMap) (*Game, *Player, error) {
	return m.Game, m.Player, m.Error
}

func (m *MockGameService) ConnectToGame(db *gorp.DbMap, gameId string, playerObj interface{}) (*Game, *Player, error) {
	return m.Game, m.Player, m.Error
}
