package main

import (
	"encoding/json"
	"flag"
	"github.com/gin-gonic/gin"
	influxdb "github.com/influxdata/influxdb/client/v2"
	"gopkg.in/redis.v3"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type Config struct {
	HttpAppHost  string
	RedisHost    string
	StaticFiles  string
	TemplateFile string
	InfluxHost   string
}

var redisClient *redis.Client
var influxClient influxdb.Client

var points []*influxdb.Point

var pointLock sync.Mutex

func loadConfig(path string) *Config {
	file, e := ioutil.ReadFile(path)

	if e != nil {
		log.Fatal("Error loading configuration file!")
	}

	var cfg Config
	json.Unmarshal(file, &cfg)

	return &cfg
}

func InfluxDBLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()

		c.Next()

		latency := time.Since(t)
		status := c.Writer.Status()

		pointLock.Lock()

		point, _ := influxdb.NewPoint("request", map[string]string{
			"host":   c.Request.Host,
			"status": strconv.Itoa(status),
		}, map[string]interface{}{
			"latency": latency.Nanoseconds(),
		}, time.Now())

		points = append(points, point)

		if len(points) > 500 {
			go func() {
				bp, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
					Database: "mcapi",
				})
				if err != nil {
					log.Println(err)
				}

				for _, point := range points {
					bp.AddPoint(point)
				}

				err = influxClient.Write(bp)
				if err != nil {
					log.Println(err)
				}

				points = []*influxdb.Point{}

				pointLock.Unlock()
			}()
		} else {
			pointLock.Unlock()
		}
	}
}

func generateConfig(path string) {
	cfg := &Config{
		HttpAppHost:  ":8080",
		RedisHost:    ":6379",
		StaticFiles:  "./scripts",
		TemplateFile: "./templates/index.html",
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

	pointLock = sync.Mutex{}

	i, err := influxdb.NewUDPClient(influxdb.UDPConfig{
		Addr: cfg.InfluxHost,
	})
	if err != nil {
		log.Println(err)
	}

	influxClient = i

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

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(InfluxDBLogger())

	router.Static("/scripts", cfg.StaticFiles)
	router.LoadHTMLFiles(cfg.TemplateFile)

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET")
		c.Writer.Header().Set("Cache-Control", "max-age=300, public, s-maxage=300")

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

	router.Run(cfg.HttpAppHost)
}
