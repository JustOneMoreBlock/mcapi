package main

import (
	"encoding/json"
	"flag"
	"github.com/lukevers/mc/mcquery"
	"github.com/syfaro/mcapi/types"
	"github.com/syfaro/minepong"
	"gopkg.in/redis.v3"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RedisHost string
}

var redisClient *redis.Client

func loadConfig(path string) *Config {
	file, e := ioutil.ReadFile(path)

	if e != nil {
		log.Fatal("Error loading configuration file!")
	}

	var cfg Config
	json.Unmarshal(file, &cfg)

	return &cfg
}

func generateConfig(path string) {
	cfg := &Config{
		RedisHost: ":6379",
	}

	data, _ := json.MarshalIndent(cfg, "", "	")

	ioutil.WriteFile(path, data, 0644)
}

var fatalServerErrors []string = []string{
	"no such host",
	"no route",
	"unknown port",
	"too many colons in address",
	"invalid argument",
}

func updateServers() {
	servers, err := redisClient.SMembers("serverping").Result()
	if err != nil {
		log.Println("Unable to get saved servers!")
	}

	log.Printf("%d servers in ping database\n", len(servers))

	for _, server := range servers {
		go updatePing(server)
	}

	servers, err = redisClient.SMembers("serverquery").Result()
	if err != nil {
		log.Println("Unable to get saved servers!")
	}

	log.Printf("%d servers in query database\n", len(servers))

	for _, server := range servers {
		go updateQuery(server)
	}
}

func updatePing(serverAddr string) *types.ServerStatus {
	var online bool
	var veryOld bool
	var status *types.ServerStatus

	online = true
	veryOld = false

	resp, err := redisClient.Get("offline:" + serverAddr).Result()
	if resp == "1" {
		status = &types.ServerStatus{}

		status.Status = "success"
		status.Online = false

		return status
	}

	resp, err = redisClient.Get("ping:" + serverAddr).Result()
	if err != nil {
		status = &types.ServerStatus{}
	} else {
		json.Unmarshal([]byte(resp), &status)
	}

	status.Error = ""

	var conn net.Conn
	if online {
		conn, err = net.DialTimeout("tcp", serverAddr, 2*time.Second)
		if err != nil {
			isFatal := false
			errString := err.Error()
			for _, e := range fatalServerErrors {
				if strings.Contains(errString, e) {
					isFatal = true
				}
			}

			if isFatal {
				redisClient.SRem("serverping", serverAddr)
				redisClient.Del("ping:" + serverAddr)

				status.Status = "error"
				status.Error = "invalid hostname or port"
				status.Online = false

				redisClient.Set("offline:"+serverAddr, "1", time.Minute)

				return status
			}

			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)

			redisClient.Set("offline:"+serverAddr, "1", time.Minute)
		}
	}

	redisClient.SAdd("serverping", serverAddr)

	var pong *minepong.Pong
	if online {
		pong, err = minepong.Ping(conn, serverAddr)
		if err != nil {
			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)

			redisClient.Set("offline:"+serverAddr, "1", time.Minute)
		}
	}

	if online {
		status.Status = "success"
		status.Online = true
		switch desc := pong.Description.(type) {
		case string:
			status.Motd = desc
		case map[string]interface{}:
			if val, ok := desc["text"]; ok {
				status.Motd = val.(string)
			}
		default:
			log.Printf("strange motd on server %s\n", serverAddr)
			log.Printf("%v", pong.Description)
			status.Motd = ""
		}
		status.Players.Max = pong.Players.Max
		status.Players.Now = pong.Players.Online
		status.Server.Name = pong.Version.Name
		status.Server.Protocol = pong.Version.Protocol
		status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
		status.LastOnline = strconv.FormatInt(time.Now().Unix(), 10)
		status.Error = ""
	} else {
		i, err := strconv.ParseInt(status.LastOnline, 10, 64)
		if err != nil {
			i = time.Now().Unix()
		}

		if time.Unix(i, 0).Add(24 * time.Hour).Before(time.Now()) {
			veryOld = true
			log.Printf("Very old server %s in database\n", serverAddr)
		}
	}

	data, err := json.Marshal(status)
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to jsonify server status)"
	}

	_, err = redisClient.Set("ping:"+serverAddr, string(data), 6*time.Hour).Result()
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
		log.Println(err.Error())
	}

	if veryOld || status.LastOnline == "" {
		redisClient.SRem("serverping", serverAddr)
		redisClient.Del("ping:" + serverAddr)
	}

	return status
}

func updateQuery(serverAddr string) *types.ServerQuery {
	online := true
	veryOld := false
	var status *types.ServerQuery

	resp, err := redisClient.Get("query:" + serverAddr).Result()
	if err != nil {
		status = &types.ServerQuery{}
	} else {
		json.Unmarshal([]byte(resp), &status)
	}

	status.Error = ""

	var conn *mcquery.Connection
	if online {
		conn, err = mcquery.Connect(serverAddr)
		if err != nil {
			isFatal := false
			errString := err.Error()
			for _, e := range fatalServerErrors {
				if strings.Contains(errString, e) {
					isFatal = true
				}
			}

			if isFatal {
				redisClient.SRem("serverquery", serverAddr)
				redisClient.Del("query:" + serverAddr)

				status.Status = "error"
				status.Error = "invalid hostname or port"
				status.Online = false

				return status
			}

			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
		}
	}

	redisClient.SAdd("serverquery", serverAddr)

	var query *mcquery.Stat
	if online {
		query, err = conn.FullStat()
		if err != nil {
			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
		}
	}

	if online {
		status.Status = "success"
		status.Online = true
		status.Motd = query.MOTD
		status.Version = query.Version
		status.GameType = query.GameType
		status.GameID = query.GameID
		status.ServerMod = query.ServerMod
		status.Map = query.Map
		status.Plugins = query.Plugins
		status.Players = types.ServerQueryPlayers{}
		status.Players.Max = query.MaxPlayers
		status.Players.Now = query.NumPlayers
		status.Players.List = query.Players
		status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
		status.LastOnline = strconv.FormatInt(time.Now().Unix(), 10)
	} else {
		i, err := strconv.ParseInt(status.LastOnline, 10, 64)
		if err != nil {
			i = time.Now().Unix()
		}

		if time.Unix(i, 0).Add(24 * time.Hour).Before(time.Now()) {
			veryOld = true
			log.Printf("Very old server %s in database\n", serverAddr)
		}
	}

	data, err := json.Marshal(status)
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to jsonify server status)"
	}

	_, err = redisClient.Set("query:"+serverAddr, string(data), 6*time.Hour).Result()
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
	}

	if veryOld || status.LastOnline == "" {
		redisClient.SRem("serverquery", serverAddr)
		redisClient.Del("query:" + serverAddr)
	}

	return status
}

func main() {
	configFile := flag.String("config", "config.json", "path to configuration file")
	genConfig := flag.Bool("gencfg", false, "generate configuration file with sane defaults")

	flag.Parse()

	f, _ := os.OpenFile("mcapi.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()

	log.SetOutput(io.MultiWriter(f, os.Stdout))

	if *genConfig {
		generateConfig(*configFile)
		log.Println("Saved configuration file with sane defaults, please update as needed")
		os.Exit(0)
	}

	cfg := loadConfig(*configFile)

	redisClient = redis.NewClient(&redis.Options{
		Addr: cfg.RedisHost,
	})

	log.Println("Updating saved servers")
	go updateServers()
	go func() {
		t := time.NewTicker(5 * time.Minute)

		for _ = range t.C {
			log.Println("Updating saved servers")
			updateServers()
		}
	}()

}
