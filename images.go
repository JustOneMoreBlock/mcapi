package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"

	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/font/inconsolata"
)

const (
	imageWidth  = 300
	imageHeight = 75
)

func respondServerImage(c *gin.Context) {
	c.Request.ParseForm()

	ip := c.Request.Form.Get("ip")
	port := c.Request.Form.Get("port")
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

	status := getStatusFromCacheOrUpdate(serverAddr)

	blockFile, err := os.Open("files/grass_sm.png")
	if err != nil {
		c.Error(err)
		return
	}

	block, _, err := image.Decode(blockFile)
	if err != nil {
		c.Error(err)
		return
	}

	bounds := block.Bounds()
	height, _ := bounds.Dy(), bounds.Dx()

	dc := gg.NewContext(imageWidth, imageHeight)

	dc.DrawImage(block, (imageHeight-height)/2, 15)

	dc.SetFontFace(inconsolata.Regular8x16)
	if theme == "dark" {
		dc.SetRGB(1, 1, 1)
	} else {
		dc.SetRGB(0, 0, 0)
	}
	_, tH := dc.MeasureString(serverDisp)
	dc.DrawString(serverDisp, 70, 18+tH)

	lastHeight := 18 + tH

	var online string

	if status.Online {
		online = "Online!"
	} else {
		online = "Offline"
	}

	tW, tH := dc.MeasureString(online)
	dc.DrawString(online, 70, lastHeight+tH+2)

	lastHeight += tH + 2

	if status.Online {
		msg := fmt.Sprintf("%d/%d players", status.Players.Now, status.Players.Max)
		_, tH = dc.MeasureString(msg)
		dc.DrawString(msg, 70+tW+5, lastHeight)
	}

	tW, tH = dc.MeasureString("mcapi.us")
	dc.DrawString("mcapi.us", imageWidth-tW-2, imageHeight-2)

	dc.EncodePNG(c.Writer)
}
