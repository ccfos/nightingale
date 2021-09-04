package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/pkg/iaop"
)

var srv = &http.Server{
	ReadTimeout:    30 * time.Second,
	WriteTimeout:   30 * time.Second,
	MaxHeaderBytes: 1 << 30,
}

var skipPaths = []string{
	"/api/n9e/auth/login",
	"/api/n9e/self/password",
	"/api/n9e/push",
	"/v1/n9e/series",
}

func Start() {
	c := config.Config

	loggerMid := iaop.LoggerWithConfig(iaop.LoggerConfig{SkipPaths: skipPaths})
	recoveryMid := iaop.Recovery()

	if strings.ToLower(c.HTTP.Mode) == "release" {
		gin.SetMode(gin.ReleaseMode)
		iaop.DisableConsoleColor()
	}

	r := gin.New()
	r.Use(recoveryMid)

	// whether print access log
	if c.HTTP.Access {
		r.Use(loggerMid)
	}

	// use cookie to save session
	store := cookie.NewStore([]byte(config.Config.HTTP.CookieSecret))
	store.Options(sessions.Options{
		Domain:   config.Config.HTTP.CookieDomain,
		MaxAge:   config.Config.HTTP.CookieMaxAge,
		Secure:   config.Config.HTTP.CookieSecure,
		HttpOnly: config.Config.HTTP.CookieHttpOnly,
		Path:     "/",
	})
	session := sessions.Sessions(config.Config.HTTP.CookieName, store)
	r.Use(session)

	configRoutes(r)
	configNoRoute(r)

	srv.Addr = c.HTTP.Listen
	srv.Handler = r

	go func() {
		fmt.Println("http.listening:", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("listening %s occur error: %s\n", srv.Addr, err)
			os.Exit(3)
		}
	}()
}

// Shutdown http server
func Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Println("cannot shutdown http server:", err)
		os.Exit(2)
	}

	// catching ctx.Done(). timeout of 5 seconds.
	select {
	case <-ctx.Done():
		fmt.Println("shutdown http server timeout of 5 seconds.")
	default:
		fmt.Println("http server stopped")
	}
}

func configNoRoute(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		arr := strings.Split(c.Request.URL.Path, ".")
		suffix := arr[len(arr)-1]
		switch suffix {
		case "png", "jpeg", "jpg", "svg", "ico", "gif", "css", "js", "html", "htm", "gz", "map":
			c.File(path.Join(strings.Split("pub/"+c.Request.URL.Path, "/")...))
		default:
			c.File(path.Join("pub", "index.html"))
		}
	})
}
