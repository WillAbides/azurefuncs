package helloworld

import (
	"fmt"
	"net/http"
)

// Handler returns a hello world handler
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		name := req.URL.Query().Get("name")
		if name == "" {
			name = "world"
		}
		fmt.Fprintf(w, "Hello %s", name)
	}
}
