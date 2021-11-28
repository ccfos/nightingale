package httpx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Host             string
	Port             int
	CertFile         string
	KeyFile          string
	PProf            bool
	PrintAccessLog   bool
	ShutdownTimeout  int
	MaxContentLength int64
	ReadTimeout      int
	WriteTimeout     int
	IdleTimeout      int
}

func Init(cfg Config, handler http.Handler) func() {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	go func() {
		fmt.Println("http server listening on:", addr)

		var err error
		if cfg.CertFile != "" && cfg.KeyFile != "" {
			srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(cfg.ShutdownTimeout))
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Println("cannot shutdown http server:", err)
		}

		select {
		case <-ctx.Done():
			fmt.Println("http exiting")
		default:
			fmt.Println("http server stopped")
		}
	}
}
