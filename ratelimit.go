package main

import (
	"time"

	"github.com/OneOfOne/cmap/stringcmap"
)

const rateLimitThreshold = 5

var rateLimit *stringcmap.CMap

func init() {
	rateLimit = stringcmap.New()

	go processRateLimit()
}

func processRateLimit() {
	for range time.Tick(time.Second) {
		rateLimit.ForEach(func(ip string, val interface{}) bool {
			i, ok := val.(int)

			if !ok {
				return true
			}

			i -= rateLimitThreshold

			if i <= 0 {
				rateLimit.Delete(ip)
			} else {
				rateLimit.Set(ip, i)
			}

			return true
		})
	}
}

func shouldRateLimit(ip string) (bool, int) {
	item := rateLimit.Get(ip)

	if item == nil {
		return false, -1
	}

	if i, ok := item.(int); ok {
		if i > rateLimitThreshold {
			incrRateLimit(ip)
			return true, i
		}
	}

	return false, -1
}

func incrRateLimit(ip string) {
	item := rateLimit.Get(ip)

	if item == nil {
		rateLimit.Set(ip, 1)
	} else if i, ok := item.(int); ok {
		rateLimit.Set(ip, i + 1)
	}
}
