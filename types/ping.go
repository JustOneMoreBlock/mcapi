package types

// ServerStatusPlayers contains information about the min and max numbers of players
// As it is a ping request, it does not contain a list of players online.
type ServerStatusPlayers struct {
	Max int `json:"max"`
	Now int `json:"now"`
}

// ServerStatusServer contains information about the server version.
// As it is a ping request, it is fairly basic information.
type ServerStatusServer struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type MotdExtra struct {
	Bold  bool   `json:"bold"`
	Color string `json:"color"`
	Text  string `json:"text"`
}

// ServerStatus contains all information available from a ping request.
// It also includes fields about the success of a request.
type ServerStatus struct {
	Status        string              `json:"status"`
	Online        bool                `json:"online"`
	Motd          string              `json:"motd"`
	MotdExtra     interface{}         `json:"motd_extra,omitempty"`
	MotdFormatted string              `json:"motd_formatted,omitempty"`
	Error         string              `json:"error"`
	Players       ServerStatusPlayers `json:"players"`
	Server        ServerStatusServer  `json:"server"`
	LastOnline    string              `json:"last_online"`
	LastUpdated   string              `json:"last_updated"`
	Duration      int64               `json:"duration"`
}
