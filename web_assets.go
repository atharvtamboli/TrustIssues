package main

import (
	"bytes"
	"embed"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed web/*.html web/*.js web/*.css
var portalAssets embed.FS

func asset(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := portalAssets.ReadFile("web/" + name)
		if err != nil {
			fail(c, 404, "Page unavailable")
			return
		}
		http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(data))
	}
}
