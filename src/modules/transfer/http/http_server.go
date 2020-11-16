package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/middleware"
	"github.com/didi/nightingale/src/modules/transfer/config"
)

var srv = &http.Server{
	ReadTimeout:    10 * time.Second,
	WriteTimeout:   10 * time.Second,
	MaxHeaderBytes: 1 << 20,
}

func Start() {
	c := config.Config

	recoveryMid := middleware.Recovery()

	if strings.ToLower(c.HTTP.Mode) == "release" {
		gin.SetMode(gin.ReleaseMode)
		middleware.DisableConsoleColor()
	}

	r := gin.New()
	r.Use(recoveryMid)

	Config(r)

	srv.Addr = address.GetHTTPListen("transfer")
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
