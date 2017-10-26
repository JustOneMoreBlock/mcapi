package main

import (
	"bytes"
	"encoding/json"
	"github.com/garyburd/redigo/redis"
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
	var status = &types.ServerStatus{}

	online = true
	veryOld = false

	t := time.Now()

	r := redisPool.Get()
	defer r.Close()

	resp, err := redis.String(r.Do("GET", "offline:"+serverAddr))

	if resp == "1" {
		status = &types.ServerStatus{}

		status.Status = "success"
		status.Online = false

		return status
	}

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
				r.Do("SREM", "serverping", serverAddr)
				r.Do("DEL", "ping:"+serverAddr)

				status.Status = "error"
				status.Error = "invalid hostname or port"
				status.Online = false

				r.Do("SETEX", "offline:"+serverAddr, 60, "1")

				return status
			}

			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)

			r.Do("SETEX", "offline:"+serverAddr, 60, "1")
		}
	}

	r.Do("SADD", "serverping", serverAddr)

	var pong *minepong.Pong
	if online {
		pong, err = minepong.Ping(conn, serverAddr)
		if err != nil {
			online = false
			status.Status = "success"
			status.Online = false
			status.LastUpdated = strconv.FormatInt(time.Now().Unix(), 10)

			r.Do("SETEX", "offline:"+serverAddr, 60, "1")
		}
	}

	if online {
		status.Status = "success"
		status.Online = true
		switch desc := pong.Description.(type) {
		case string:
			status.Motd = desc
		case map[string]interface{}:
			if val, ok := desc["extra"]; ok {
				texts := val.([]interface{})

				b := bytes.Buffer{}
				f := bytes.Buffer{}

				f.WriteString("<span>")

				for id, text := range texts {
					m := text.(map[string]interface{})
					extra := types.MotdExtra{}

					for k, v := range m {
						if k == "text" {
							b.WriteString(v.(string))
							extra.Text = v.(string)
						} else if k == "color" {
							extra.Color = v.(string)
						} else if k == "bold" {
							extra.Bold = v.(bool)
						}
					}

					f.WriteString("<span")

					if extra.Color != "" || extra.Bold {
						f.WriteString(" style='")

						if extra.Color != "" {
							f.WriteString("color: ")
							f.WriteString(extra.Color)
							f.WriteString("; ")
						}

						if extra.Bold {
							f.WriteString(" font-weight: bold; ")
						}

						f.WriteString("'")
					}

					f.WriteString(">")
					f.WriteString(extra.Text)
					f.WriteString("</span>")

					if id != len(texts)-1 {
						f.WriteString(" ")
					}
				}

				f.WriteString("</span>")

				status.Motd = b.String()
				status.MotdExtra = val
				status.MotdFormatted = strings.Replace(f.String(), "\n", "<br>", -1)
			} else if val, ok := desc["text"]; ok {
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

	_, err = r.Do("SETEX", "ping:"+serverAddr, 6*60*60, string(data))
	if err != nil {
		status.Status = "error"
		status.Error = "internal server error (unable to save json to redis)"
		raven.CaptureErrorAndWait(err, nil)
	}

	if veryOld || status.LastOnline == "" {
		r.Do("SREM", "serverping", serverAddr)
		r.Do("DEL", "ping:"+serverAddr)
	}

	return status
}

func getServerStatusFromRedis(serverAddr string) *types.ServerStatus {
	r := redisPool.Get()
	resp, err := redis.String(r.Do("GET", "ping:"+serverAddr))
	r.Close()

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
