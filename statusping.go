package main

import (
	"encoding/json"
	"github.com/getsentry/raven-go"
	"github.com/gin-gonic/gin"
	"github.com/syfaro/mcapi/types"
	"github.com/syfaro/minepong"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func updatePing(serverAddr string) *types.ServerStatus {
	var online bool
	var veryOld bool
	var status *types.ServerStatus

	online = true
	veryOld = false

	t := time.Now()

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

	i, _ := strconv.ParseInt(status.LastUpdated, 10, 64)
	if time.Unix(i, 0).Add(5 * time.Minute).After(time.Now()) {
		return status
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

	diff := time.Since(t)

	status.Duration = diff.Nanoseconds()

	data, err := json.Marshal(status)
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to jsonify server status)"
		raven.CaptureErrorAndWait(err, nil)
	}

	_, err = redisClient.Set("ping:"+serverAddr, string(data), 6*time.Hour).Result()
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
		raven.CaptureErrorAndWait(err, nil)
	}

	if veryOld || status.LastOnline == "" {
		redisClient.SRem("serverping", serverAddr)
		redisClient.Del("ping:" + serverAddr)
	}

	return status
}

func getServerStatusFromRedis(serverAddr string) *types.ServerStatus {
	resp, err := redisClient.Get("ping:" + serverAddr).Result()
	if err != nil {
		status := updatePing(serverAddr)

		return status
	}

	var status types.ServerStatus
	err = json.Unmarshal([]byte(resp), &status)
	if err != nil {
		return &types.ServerStatus{
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
		c.JSON(http.StatusBadRequest, &types.ServerStatus{
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
