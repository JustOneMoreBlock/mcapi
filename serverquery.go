package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/syfaro/mc/mcquery"
	"github.com/syfaro/mcapi/types"
)

func updateQuery(serverAddr string) *types.ServerQuery {
	log.Printf("Querying %s\n", serverAddr)

	var online bool
	var veryOld bool
	var status = &types.ServerQuery{}

	online = true
	veryOld = false

	t := time.Now()

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
				queryMap.Delete(serverAddr)

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

	queryMap.Set(serverAddr, status)

	if veryOld {
		queryMap.Delete(serverAddr)
	}

	return status
}

func getQueryFromCacheOrUpdate(serverAddr string) *types.ServerQuery {
	serverAddr = strings.ToLower(serverAddr)

	if status, ok := queryMap.GetOK(serverAddr); ok {
		return status.(*types.ServerQuery)
	}

	return updateQuery(serverAddr)
}

func respondServerQuery(c *gin.Context) {
	c.Request.ParseForm()

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

	var serverAddr string

	if port == "" {
		serverAddr = ip + ":25565"
	} else {
		serverAddr = ip + ":" + port
	}

	c.JSON(http.StatusOK, getQueryFromCacheOrUpdate(serverAddr))
}
