package routes

import (
	"fmt"
	"net/http"
	"os"
)

func ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

func addr(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, r.RemoteAddr)
}

func pid(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, fmt.Sprintf("%d", os.Getpid()))
}
