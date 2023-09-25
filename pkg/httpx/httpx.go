package httpx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"path/filepath"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/ccfos/nightingale/v6/pkg/version"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Host             string
	Port             int
	CertFile         string
	KeyFile          string
	PProf            bool
	PrintAccessLog   bool
	ExposeMetrics    bool
	ShutdownTimeout  int
	MaxContentLength int64
	ReadTimeout      int
	WriteTimeout     int
	IdleTimeout      int
	JWTAuth          JWTAuth
	ProxyAuth        ProxyAuth
	ShowCaptcha      ShowCaptcha
	APIForAgent      BasicAuths
	APIForService    BasicAuths
	RSA              RSAConfig
}

type RSAConfig struct {
	OpenRSA           bool
	RSAPublicKey      []byte
	RSAPublicKeyPath  string
	RSAPrivateKey     []byte
	RSAPrivateKeyPath string
	RSAPassWord       string
}

type ShowCaptcha struct {
	Enable bool
}

type BasicAuths struct {
	BasicAuth gin.Accounts
	Enable    bool
}

type ProxyAuth struct {
	Enable            bool
	HeaderUserNameKey string
	DefaultRoles      []string
}

type JWTAuth struct {
	SigningKey     string
	AccessExpired  int64
	RefreshExpired int64
	RedisKeyPrefix string
}

func GinEngine(mode string, cfg Config) *gin.Engine {
	gin.SetMode(mode)

	loggerMid := aop.Logger()
	recoveryMid := aop.Recovery()

	if strings.ToLower(mode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()

	r.Use(recoveryMid)

	// whether print access log
	if cfg.PrintAccessLog {
		r.Use(loggerMid)
	}

	if cfg.PProf {
		pprof.Register(r, "/api/debug/pprof")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/pid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getpid()))
	})

	r.GET("/ppid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getppid()))
	})

	r.GET("/addr", func(c *gin.Context) {
		c.String(200, c.Request.RemoteAddr)
	})

	r.GET("/api/n9e/version", func(c *gin.Context) {
		c.String(200, version.Version)
	})

	if cfg.ExposeMetrics {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	return r
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

func InitRSAConfig(rsaConfig *RSAConfig) {
	if err := initRSAFile(rsaConfig); err != nil {
		logger.Warning(err)
	}
	// 读取公钥配置文件
	//获取文件内容
	publicBuf, err := os.ReadFile(rsaConfig.RSAPublicKeyPath)
	if err != nil {
		logger.Warningf("could not read RSAPublicKeyPath %s: %v", rsaConfig.RSAPublicKeyPath, err)
	}
	rsaConfig.RSAPublicKey = publicBuf
	// 读取私钥配置文件
	privateBuf, err := os.ReadFile(rsaConfig.RSAPrivateKeyPath)
	if err != nil {
		logger.Warningf("could not read RSAPrivateKeyPath %s: %v", rsaConfig.RSAPrivateKeyPath, err)
	}
	rsaConfig.RSAPrivateKey = privateBuf
}

func initRSAFile(encryption *RSAConfig) error {
	dirPath := filepath.Dir(encryption.RSAPrivateKeyPath)
	// Check if the directory exists
	errCreateDir := file.InsureDir(dirPath)
	if errCreateDir != nil {
		return fmt.Errorf("could not create directory for initRSAFile %q: %v", dirPath, errCreateDir)
	}
	// Check if the file exists
	if file.IsExist(encryption.RSAPrivateKeyPath) {
		errGen := secu.GenerateKeyWithPassword(encryption.RSAPrivateKeyPath, encryption.RSAPublicKeyPath, encryption.RSAPassWord)
		if errGen != nil {
			return fmt.Errorf("could not create file for initRSAFile %+v: %v", encryption, errGen)
		}
	}
	return nil
}
