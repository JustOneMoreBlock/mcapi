package main

import (
	"bytes"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/syfaro/mcapi/types"
	"github.com/syfaro/minepong"
)

func updatePing(serverAddr string) *types.ServerStatus {
	log.Printf("Pinging %s\n", serverAddr)

	var online bool
	var veryOld bool
	var status = &types.ServerStatus{}

	online = true
	veryOld = false

	t := time.Now()

	pong, err := minepong.Ping(serverAddr)

	if err != nil {
		isFatal := false
		errString := err.Error()
		for _, e := range fatalServerErrors {
			if strings.Contains(errString, e) {
				isFatal = true
			}
		}

		if isFatal {
			pingMap.Delete(serverAddr)

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
		status.Favicon = pong.FavIcon
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

	pingMap.Set(serverAddr, status)

	if veryOld {
		pingMap.Delete(serverAddr)
	}

	return status
}

func getStatusFromCacheOrUpdate(serverAddr string, c *gin.Context, hideError bool) *types.ServerStatus {
	serverAddr = strings.ToLower(serverAddr)

	if status, ok := pingMap.GetOK(serverAddr); ok {
		return status.(*types.ServerStatus)
	}

	ip := c.GetHeader("CF-Connecting-IP")

	log.Printf("New server %s from %s\n", serverAddr, ip)

	if limit, count := shouldRateLimit(ip); limit {
		if !hideError {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, struct {
				Error    string `json:"error"`
				TryAfter int    `json:"try_after"`
			}{
				Error:    "too many invalid requests",
				TryAfter: count / rateLimitThreshold,
			})
		}

		return nil
	}

	status := updatePing(serverAddr)

	if status.Error != "" {
		incrRateLimit(ip)
	}

	return status
}

func respondServerStatus(c *gin.Context) {
	c.Request.ParseForm()

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

	var serverAddr string

	if port == "" {
		serverAddr = ip + ":25565"
	} else {
		serverAddr = ip + ":" + port
	}

	status := getStatusFromCacheOrUpdate(serverAddr, c, false)

	if status == nil {
		return
	}

	c.JSON(http.StatusOK, status)
}
