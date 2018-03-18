package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/OneOfOne/cmap/stringcmap"
	"github.com/garyburd/redigo/redis"
	"github.com/getsentry/raven-go"
	"github.com/gin-gonic/gin"
	"github.com/gocraft/work"
	"github.com/syfaro/mcapi/types"
)

type Config struct {
	HttpAppHost  string
	RedisHost    string
	StaticFiles  string
	TemplateFile string
	SentryDSN    string
	AdminKey     string
}

var redisPool *redis.Pool

var enqueuer *work.Enqueuer

var pingMap *stringcmap.CMap
var queryMap *stringcmap.CMap

func loadConfig(path string) *Config {
	file, err := ioutil.ReadFile(path)

	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		panic(err)
	}

	var cfg Config
	json.Unmarshal(file, &cfg)

	return &cfg
}

func generateConfig(path string) {
	cfg := &Config{
		HttpAppHost:  ":8080",
		RedisHost:    ":6379",
		StaticFiles:  "./scripts",
		TemplateFile: "./templates/index.html",
		AdminKey:     "your_secret",
	}

	data, err := json.MarshalIndent(cfg, "", "	")
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
	}

	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
	}
}

var fatalServerErrors = []string{
	"no such host",
	"no route",
	"unknown port",
	"too many colons in address",
	"invalid argument",
}

func updateServers() {
	pingMap.ForEachLocked(func(key string, _ interface{}) bool {
		enqueuer.Enqueue("status", work.Q{"serverAddr": key})

		return true
	})

	queryMap.ForEachLocked(func(key string, _ interface{}) bool {
		enqueuer.Enqueue("query", work.Q{"serverAddr": key})

		return true
	})
}

type JobCtx struct{}

func jobMiddleware(job *work.Job, next work.NextMiddlewareFunc) error {
	log.Printf("Running %s: %+v\n", job.Name, job.Args)
	return next()
}

func jobUpdateQuery(job *work.Job) error {
	if _, ok := job.Args["serverAddr"]; ok {
		serverAddr := job.ArgString("serverAddr")

		res := updateQuery(serverAddr)
		if res.Error != "" {
			return errors.New(res.Error)
		} else {
			return nil
		}
	} else {
		return errors.New("missing server address")
	}
}

func jobUpdateStatus(job *work.Job) error {
	if _, ok := job.Args["serverAddr"]; ok {
		serverAddr := job.ArgString("serverAddr")

		res := updatePing(serverAddr)
		if res.Error != "" {
			return errors.New(res.Error)
		} else {
			return nil
		}
	} else {
		return errors.New("missing server address")
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

	raven.SetDSN(cfg.SentryDSN)

	redisPool = &redis.Pool{
		MaxActive:   200,
		MaxIdle:     100,
		Wait:        true,
		IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", cfg.RedisHost)
		},
	}

	pingMap = stringcmap.New()
	queryMap = stringcmap.New()

	enqueuer = work.NewEnqueuer("mcapi", redisPool)

	pool := work.NewWorkerPool(JobCtx{}, 50, "mcapi", redisPool)

	pool.Middleware(jobMiddleware)

	pool.Job("query", jobUpdateQuery)
	pool.Job("status", jobUpdateStatus)

	go pool.Start()

	updateServers()
	go func() {
		for range time.Tick(5 * time.Minute) {
			updateServers()
		}
	}()

	router := gin.New()
	router.Use(gin.Recovery())

	router.Static("/scripts", cfg.StaticFiles)
	router.LoadHTMLFiles(cfg.TemplateFile)

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET")
		c.Writer.Header().Set("Cache-Control", "max-age=300, public, s-maxage=300")

		r := redisPool.Get()
		r.Do("INCR", "mcapi")
		r.Close()
	})

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	router.GET("/stats", func(c *gin.Context) {
		r := redisPool.Get()
		stats, err := redis.Int64(r.Do("GET", "mcapi"))
		r.Close()

		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
		}

		c.JSON(http.StatusOK, gin.H{
			"stats": stats,
			"time":  time.Now().UnixNano(),
		})
	})

	router.GET("/server/status", respondServerStatus)
	router.GET("/minecraft/1.3/server/status", respondServerStatus)

	router.GET("/server/query", respondServerQuery)
	router.GET("/minecraft/1.3/server/query", respondServerQuery)

	authorized := router.Group("/admin", gin.BasicAuth(gin.Accounts{
		"mcapi": cfg.AdminKey,
	}))

	authorized.GET("/ping", func(c *gin.Context) {
		items := strings.Builder{}

		pingMap.ForEachLocked(func(key string, val interface{}) bool {
			ping, ok := val.(*types.ServerStatus)
			if !ok {
				return true
			}

			items.WriteString(key)
			items.Write([]byte(" - "))
			items.WriteString(ping.LastUpdated)
			items.Write([]byte("\n"))

			return true
		})

		c.String(http.StatusOK, items.String())
	})

	authorized.GET("/query", func(c *gin.Context) {
		items := strings.Builder{}

		queryMap.ForEachLocked(func(key string, val interface{}) bool {
			ping, ok := val.(*types.ServerQuery)
			if !ok {
				return true
			}

			items.WriteString(key)
			items.Write([]byte(" - "))
			items.WriteString(ping.LastUpdated)
			items.Write([]byte("\n"))

			return true
		})

		c.String(http.StatusOK, items.String())
	})

	authorized.POST("/clear", func(c *gin.Context) {
		pingMap.ForEach(func(key string, _ interface{}) bool {
			pingMap.Delete(key)
			return true
		})

		queryMap.ForEach(func(key string, _ interface{}) bool {
			queryMap.Delete(key)
			return true
		})

		c.String(http.StatusOK, "Cleared items.")
	})

	router.Run(cfg.HttpAppHost)
}
