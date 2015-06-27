package main

import (
	"encoding/json"
	"flag"
	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
	"gopkg.in/redis.v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

func updateServers() {
	servers, err := redisClient.SMembers("servers").Result()
	if err != nil {
		log.Println("Unable to get saved servers!")
	}

	log.Printf("%d servers in database\n", len(servers))

	for _, server := range servers {
		go updatePing(server)
	}
}

func main() {
	configFile := flag.String("config", "config.json", "path to configuration file")
	genConfig := flag.Bool("gencfg", false, "generate configuration file with sane defaults")

	flag.Parse()

	f, _ := os.OpenFile("mcapi.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()

	log.SetOutput(f)

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
			"time":  time.Now().UnixNano(),
		})
	})

	router.GET("/server/status", respondServerStatus)
	router.GET("/minecraft/1.3/server/status", respondServerStatus)

	router.GET("/server/query", respondServerQuery)
	router.GET("/minecraft/1.3/server/query", respondServerQuery)

	endless.ListenAndServe(cfg.HttpAppHost, router)
}
