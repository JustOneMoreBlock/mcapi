package main

import (
	"fmt"
	"image"
	_ "image/png"
	"strconv"
	"time"

	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/font/inconsolata"
)

const (
	imageWidth  = 325
	imageHeight = 64
)

const (
	imageBlockWidth = 64
	fromImage = 4
	offsetText = float64(imageBlockWidth + fromImage)
)

func respondServerImage(c *gin.Context) {
	c.Request.ParseForm()

	ip := c.Request.Form.Get("ip")
	port := c.Request.Form.Get("port")
	title := c.Request.Form.Get("title")
	theme := c.Request.Form.Get("theme")

	var serverAddr string
	var serverDisp string

	if port == "" {
		serverAddr = ip + ":25565"
		serverDisp = ip
	} else {
		serverAddr = ip + ":" + port
		serverDisp = serverAddr
	}

	if title != "" {
		serverDisp = title
	}

	status := getStatusFromCacheOrUpdate(serverAddr)

	var imgToDraw image.Image

	if status.Favicon == "" {
		img, err := gg.LoadPNG("files/grass_sm.png")
		if err != nil {
			c.Error(err)
			return
		}

		imgToDraw = img
	} else {
		img, err := status.Image()
		if err != nil {
			c.Error(err)
			return
		}

		imgToDraw = img
	}

	bounds := imgToDraw.Bounds()
	height, width := bounds.Dy(), bounds.Dx()

	dc := gg.NewContext(imageWidth, imageHeight)

	dc.DrawImage(imgToDraw, (imageBlockWidth-width)/2, (imageHeight-height)/2)

	dc.SetFontFace(inconsolata.Regular8x16)
	if theme == "dark" {
		dc.SetRGB(1, 1, 1)
	} else {
		dc.SetRGB(0, 0, 0)
	}
	_, tH := dc.MeasureString(serverDisp)
	dc.DrawString(serverDisp, offsetText, tH)

	lastHeight := 1 + tH

	var online string

	if status.Online {
		online = "Online!"
	} else {
		online = "Offline"
	}

	tW, tH := dc.MeasureString(online)
	dc.DrawString(online, offsetText, lastHeight+tH+2)

	lastHeight += tH + 2

	if status.Online {
		msg := fmt.Sprintf("%d/%d players", status.Players.Now, status.Players.Max)
		_, tH = dc.MeasureString(msg)
		dc.DrawString(msg, float64(width+fromImage*2)+tW, lastHeight)
	}

	i, _ := strconv.ParseInt(status.LastUpdated, 10, 64)
	last := time.Unix(i, 0)
	minutesAgo := int(time.Now().Sub(last).Minutes())

	plural := ""
	if minutesAgo != 1 {
		plural = "s"
	}

	msg := fmt.Sprintf("Updated %d min%s ago · mcapi.us", minutesAgo, plural)

	dc.DrawString(msg, offsetText, imageHeight-4)

	dc.EncodePNG(c.Writer)
}
