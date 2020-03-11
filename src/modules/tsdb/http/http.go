package http

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/didi/nightingale/src/modules/tsdb/http/middleware"
	"github.com/didi/nightingale/src/modules/tsdb/http/render"
	"github.com/didi/nightingale/src/modules/tsdb/http/routes"
	"github.com/didi/nightingale/src/toolkits/address"

	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/toolkits/pkg/logger"
)

var Close_chan, Close_done_chan chan int

func init() {
	Close_chan = make(chan int, 1)
	Close_done_chan = make(chan int, 1)
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type TcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln TcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func Start() {
	render.Init()

	r := mux.NewRouter().StrictSlash(false)
	routes.ConfigRoutes(r)

	n := negroni.New()
	n.Use(middleware.NewLogger())
	n.Use(middleware.NewRecovery())

	n.UseHandler(r)

	addr := address.GetHTTPListen("tsdb")
	if addr == "" {
		return
	}
	s := &http.Server{
		Addr:           addr,
		MaxHeaderBytes: 1 << 30,
		Handler:        n,
	}
	logger.Info("http listening", addr)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
		return
	}
	l := ln.(*net.TCPListener)
	go s.Serve(TcpKeepAliveListener{l})

	select {
	case <-Close_chan:
		log.Println("http recv sigout and exit...")
		l.Close()
		Close_done_chan <- 1
		return
	}

}
