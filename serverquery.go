package main

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	"github.com/getsentry/raven-go"
	"github.com/gin-gonic/gin"
	"github.com/syfaro/mc/mcquery"
	"github.com/syfaro/mcapi/types"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func updateQuery(serverAddr string) *types.ServerQuery {
	var online bool
	var veryOld bool
	var status = &types.ServerQuery{}

	online = true
	veryOld = false

	t := time.Now()

	r := redisPool.Get()
	defer r.Close()

	var err error
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
				r.Do("SREM", "serverquery", serverAddr)
				r.Do("DEL", "query:"+serverAddr)

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

	r.Do("SADD", "serverquery", serverAddr)

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

	diff := time.Since(t)

	status.Duration = diff.Nanoseconds()

	data, err := json.Marshal(status)
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to jsonify server status)"
		raven.CaptureErrorAndWait(err, nil)
	}

	_, err = r.Do("SETEX", "query:"+serverAddr, 6*60*60, string(data))

	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
		raven.CaptureErrorAndWait(err, nil)
	}

	if veryOld || status.LastOnline == "" {
		r.Do("SREM", "serverquery", serverAddr)
		r.Do("DEL", "query:"+serverAddr)
	}

	return status
}

func getServerQueryFromRedis(serverAddr string) *types.ServerQuery {
	r := redisPool.Get()
	resp, err := redis.String(r.Do("GET", "query:"+serverAddr))
	r.Close()

	if err != nil {
		status := updateQuery(serverAddr)

		return status
	}

	var status types.ServerQuery
	err = json.Unmarshal([]byte(resp), &status)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		return &types.ServerQuery{
			Status: "error",
			Error:  "internal server error (error loading json from redis)",
		}
	}

	return &status
}

func respondServerQuery(c *gin.Context) {
	c.Request.ParseForm()

	var serverAddr string

	ip := c.Request.Form.Get("ip")
	port := c.Request.Form.Get("port")

	if ip == "" {
		c.JSON(http.StatusBadRequest, &types.ServerQuery{
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

	c.JSON(http.StatusOK, getServerQueryFromRedis(serverAddr))
}
