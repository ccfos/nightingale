package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/http/routes"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/http/middleware"
)

var srv = &http.Server{
	ReadTimeout:    10 * time.Second,
	WriteTimeout:   10 * time.Second,
	MaxHeaderBytes: 1 << 20,
}

var skipPaths = []string{
	"/api/portal/auth/login",
	"/api/portal/self/password",
	"/api/portal/ping",
}

// Start http server
func Start() {
	c := config.Get()

	loggerMid := middleware.LoggerWithConfig(middleware.LoggerConfig{SkipPaths: skipPaths})
	recoveryMid := middleware.Recovery()
	store := cookie.NewStore([]byte(c.HTTP.Secret))
	sessionMid := sessions.Sessions("falcon-ng", store)

	if c.Logger.Level != "DEBUG" {
		gin.SetMode(gin.ReleaseMode)
		middleware.DisableConsoleColor()
	}

	r := gin.New()
	r.Use(loggerMid, recoveryMid, sessionMid)

	routes.Config(r)

	srv.Addr = address.GetHTTPListen("monapi")
	srv.Handler = r

	go func() {
		log.Println("starting http server, listening on:", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listening %s occur error: %s\n", srv.Addr, err)
		}
	}()
}

// Shutdown http server
func Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalln("cannot shutdown http server:", err)
	}

	// catching ctx.Done(). timeout of 5 seconds.
	select {
	case <-ctx.Done():
		log.Println("shutdown http server timeout of 5 seconds.")
	default:
		log.Println("http server stopped")
	}
}
