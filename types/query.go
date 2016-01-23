package types

// ServerQueryPlayers contains information about the min and max numbers of players online.
// It also includes a list of players online, unlike a ping request.
type ServerQueryPlayers struct {
	Max  int      `json:"max"`
	Now  int      `json:"now"`
	List []string `json:"list"`
}

// ServerQuery contains all information available from a query request to a server.
// This is the most specific information you can easily get from a server.
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
	Duration    int64              `json:"duration"`
}
