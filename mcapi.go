package main

import (
	"encoding/json"
	"flag"
	"github.com/andrewtian/minepong"
	"github.com/dcu/http-einhorn"
	"github.com/gin-gonic/gin"
	"gopkg.in/redis.v3"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HttpAppHost   string
	RedisHost     []string
	StaticFiles   string
	TemplateFiles string
}

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

var redisClient *redis.ClusterClient

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
		HttpAppHost:   ":8080",
		RedisHost:     []string{":7000", ":7001"},
		StaticFiles:   "./scripts",
		TemplateFiles: "./templates/*",
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

func updateHost(serverAddr string) *ServerStatus {
	var online bool
	var veryOld bool
	var status *ServerStatus

	online = true
	veryOld = false

	resp, err := redisClient.Get("offline:" + serverAddr).Result()
	if resp == "1" {
		status = &ServerStatus{}

		status.Status = "success"
		status.Online = false

		return status
	}

	resp, err = redisClient.Get(serverAddr).Result()
	if err != nil {
		status = &ServerStatus{}
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
				redisClient.SRem("servers", serverAddr)
				redisClient.Del(serverAddr)

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

	redisClient.SAdd("servers", serverAddr)

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

	_, err = redisClient.Set(serverAddr, string(data[:]), 6*time.Hour).Result()
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
	}

	if veryOld || status.LastOnline == "" {
		redisClient.SRem("servers", serverAddr)
		redisClient.Del(serverAddr)
	}

	return status
}

func updateServers() {
	servers, err := redisClient.SMembers("servers").Result()
	if err != nil {
		log.Println("Unable to get saved servers!")
	}

	log.Printf("%d servers in database\n", len(servers))

	for _, server := range servers {
		go updateHost(server)
	}
}

func getServerStatusFromRedis(serverAddr string) *ServerStatus {
	resp, err := redisClient.Get(serverAddr).Result()
	if err != nil {
		status := updateHost(serverAddr)

		return status
	}

	var status ServerStatus
	err = json.Unmarshal([]byte(resp), &status)
	if err != nil {
		return &ServerStatus{
			Status: "error",
			Error:  "internal server error (error loading json from redis)",
		}
	}

	return &status
}

func respondServerStatus(c *gin.Context) {
	c.Request.ParseForm()

	var serverAddr string

	ip := c.Request.Form.Get("ip")
	port := c.Request.Form.Get("port")

	if ip == "" {
		c.JSON(http.StatusBadRequest, &ServerStatus{
			Online: false,
			Status: "error",
			Error:  "missing data",
		})
		return
	}

	if port == "" {
		serverAddr = ip + ":25565"
	} else {
		serverAddr = ip + ":" + port
	}

	c.JSON(http.StatusOK, getServerStatusFromRedis(serverAddr))
}

func main() {
	configFile := flag.String("config", "config.json", "path to configuration file")
	genConfig := flag.Bool("gencfg", false, "generate configuration file with sane defaults")

	flag.Parse()

	if *genConfig {
		generateConfig(*configFile)
		log.Println("Saved configuration file with sane defaults, please update as needed")
		os.Exit(0)
	}

	cfg := loadConfig(*configFile)

	redisClient = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: cfg.RedisHost,
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

	router := gin.New()
	router.Use(gin.Recovery())

	router.Static("/scripts", cfg.StaticFiles)
	router.LoadHTMLGlob(cfg.TemplateFiles)

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET")

		redisClient.Incr("mcapi")
	})

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	router.GET("/hi", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello :3")
	})

	router.GET("/stats", func(c *gin.Context) {
		stats, _ := redisClient.Get("mcapi").Int64()

		c.JSON(http.StatusOK, gin.H{
			"stats": stats,
			"time":  time.Now().Unix(),
		})
	})

	router.GET("/server/status", respondServerStatus)
	router.GET("/minecraft/1.3/server/status", respondServerStatus)

	if einhorn.IsRunning() {
		einhorn.Start(router, 0)
	} else {
		router.Run(cfg.HttpAppHost)
	}
}
