package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	gv_select "github.com/willabides/azurefuncs/goversion_select"
	"github.com/willabides/azurefuncs/helloworld"
	"github.com/willabides/azurefuncs/ping"
)

var version string

func main() {
	if version == "" {
		version = "dev"
	}
	listenAddr := ":9834"
	if val, ok := os.LookupEnv("FUNCTIONS_CUSTOMHANDLER_PORT"); ok {
		listenAddr = ":" + val
	}
	sMux := http.NewServeMux()
	sMux.HandleFunc("/api/helloworld", helloworld.Handler())
	sMux.HandleFunc("/api/ping", ping.Handler(version))
	sMux.Handle("/api/goversion_select", &gv_select.Handler{
		VersionsMaxAge: 15 * time.Minute,
		VersionsSource: "https://raw.githubusercontent.com/WillAbides/goreleases/main/versions.txt",
	})
	sMux.HandleFunc("/api/env", func(w http.ResponseWriter, req *http.Request) {
		for _, s := range os.Environ() {
			fmt.Fprintln(w, []byte(s))
		}
	})

	log.Printf("About to listen on %s. Go to http://127.0.0.1%s/", listenAddr, listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Println("got a request", req.URL.String())
		sMux.ServeHTTP(w, req)
	})))
}
