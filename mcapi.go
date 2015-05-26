package main

import (
	"encoding/json"
	"flag"
	"github.com/andrewtian/minepong"
	"github.com/garyburd/redigo/redis"
	"github.com/gin-gonic/gin"
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
	RedisPath     string
	RedisDatabase string
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

var redisPool *redis.Pool

func newRedisPool(server string, database string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("unix", server)
			if err != nil {
				return nil, err
			}

			if _, err := c.Do("SELECT", database); err != nil {
				c.Close()
				return nil, err
			}

			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")

			return err
		},
	}
}

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
		RedisPath:     "/tmp/redis.sock",
		RedisDatabase: "0",
		StaticFiles:   "./scripts",
		TemplateFiles: "./templates/*",
	}

	data, _ := json.MarshalIndent(cfg, "", "	")

	ioutil.WriteFile(path, data, 0644)
}

func updateHost(serverAddr string, debug bool) *ServerStatus {
	r := redisPool.Get()
	defer r.Close()

	var online bool
	var veryOld bool
	var status *ServerStatus

	online = true
	veryOld = false

	resp, err := redis.String(r.Do("GET", serverAddr))
	if err != nil {
		status = &ServerStatus{}
	} else {
		json.Unmarshal([]byte(resp), &status)
	}

	status.Error = ""

	var conn net.Conn
	if online {
		conn, err = net.Dial("tcp", serverAddr)
		if err != nil {
			if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "no route") || strings.Contains(err.Error(), "unknown port") || strings.Contains(err.Error(), "too many colons in address") || strings.Contains(err.Error(), "invalid argument") {
				log.Printf("Bad server requested: %s\n", serverAddr)

				r.Do("SREM", "servers", serverAddr)
				r.Do("DEL", serverAddr)

				status.Status = "error"
				status.Error = "invalid hostname or port"
				status.Online = false

				return status
			}

			log.Printf("Server is offline: %s, %v\n", serverAddr, err)

			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
		}
	}

	r.Do("SADD", "servers", serverAddr)

	var pong *minepong.Pong
	if online {
		pong, err = minepong.Ping(conn, serverAddr)
		if err != nil {
			log.Printf("Server does not respond to ping: %s, %v", serverAddr, err)

			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)
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

	_, err = r.Do("SET", serverAddr, data)
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
	}

	if veryOld || status.LastOnline == "" {
		r.Do("SREM", "servers", serverAddr)
		r.Do("DEL", serverAddr)
	}

	return status
}

func updateServers() {
	r := redisPool.Get()
	defer r.Close()

	servers, err := redis.Strings(r.Do("SMEMBERS", "servers"))
	if err != nil {
		log.Println("Unable to get saved servers!")
	}

	log.Printf("%d servers in database\n", len(servers))

	for _, server := range servers {
		go updateHost(server, false)
	}
}

func getServerStatusFromRedis(serverAddr string, debug bool) *ServerStatus {
	r := redisPool.Get()
	defer r.Close()

	resp, err := redis.String(r.Do("GET", serverAddr))
	if err != nil {
		if debug {
			log.Printf("Could not get value from cache for %s\n", serverAddr)
		}

		status := updateHost(serverAddr, debug)

		return status
	}

	var status ServerStatus
	err = json.Unmarshal([]byte(resp), &status)
	if err != nil {
		if debug {
			log.Printf("Unable to parse response from cache for %s\n", serverAddr)
		}

		return &ServerStatus{
			Status: "error",
			Error:  "internal server error (error loading json from redis)",
		}
	}

	if debug {
		log.Printf("Returned stats for server %s\n", serverAddr)
	}

	return &status
}

func respondServerStatus(c *gin.Context) {
	c.Request.ParseForm()

	var serverAddr string

	ip := c.Request.Form.Get("ip")
	port := c.Request.Form.Get("port")
	debug := c.Request.Form.Get("debug")

	checkDebug, _ := strconv.ParseBool(debug)

	if ip == "" {
		if checkDebug {
			log.Printf("Server request had missing IP\n")
		}

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

	if checkDebug {
		log.Printf("Got server request for %s\n", serverAddr)
	}

	c.JSON(http.StatusOK, getServerStatusFromRedis(serverAddr, checkDebug))
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

	redisPool = newRedisPool(cfg.RedisPath, cfg.RedisDatabase)

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
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET")

		r := redisPool.Get()
		defer r.Close()

		r.Do("INCR", "mcapi")

		c.Next()
	})

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	router.GET("/hi", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello :3")
	})

	router.GET("/stats", func(c *gin.Context) {
		r := redisPool.Get()
		defer r.Close()

		stats, _ := redis.Int64(r.Do("GET", "mcapi"))

		c.JSON(http.StatusOK, gin.H{
			"stats": stats,
		})
	})

	router.GET("/server/status", respondServerStatus)
	router.GET("/minecraft/1.3/server/status", respondServerStatus)

	router.Run(cfg.HttpAppHost)
}
