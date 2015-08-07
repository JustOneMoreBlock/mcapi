package mcapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	APIEndpoint = "https://mcapi.us"
)

type ServerStatusPlayers struct {
	Max int `json:"max"`
	Now int `json:"now"`
}

type ServerStatusServer struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type ServerStatus struct {
	Status      string              `json:"status"`
	Online      bool                `json:"online"`
	Motd        string              `json:"motd"`
	Error       string              `json:"error"`
	Players     ServerStatusPlayers `json:"players"`
	Server      ServerStatusServer  `json:"server"`
	LastOnline  string              `json:"last_online"`
	LastUpdated string              `json:"last_updated"`
}

type ServerQueryPlayers struct {
	Max  int      `json:"max"`
	Now  int      `json:"now"`
	List []string `json:"list"`
}

type ServerQuery struct {
	Status      string             `json:"status"`
	Online      bool               `json:"online"`
	Error       string             `json:"error"`
	Motd        string             `json:"motd"`
	Version     string             `json:"version"`
	GameType    string             `json:"game_type"`
	GameID      string             `json:"game_id"`
	ServerMod   string             `json:"server_mod"`
	Map         string             `json:"map"`
	Players     ServerQueryPlayers `json:"players"`
	Plugins     []string           `json:"plugins"`
	LastOnline  string             `json:"last_online"`
	LastUpdated string             `json:"last_updated"`
}

func GetServerStatus(ip string, port int) (*ServerStatus, error) {
	resp, err := http.Get(fmt.Sprintf("%s/server/status?ip=%s&port=%d", APIEndpoint, ip, port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)

	var status ServerStatus
	err = json.Unmarshal(data, &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func GetServerQuery(ip string, port int) (*ServerQuery, error) {
	resp, err := http.Get(fmt.Sprintf("%s/server/query?ip=%s&port=%d", APIEndpoint, ip, port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)

	var status ServerQuery
	err = json.Unmarshal(data, &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}
