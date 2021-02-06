package ping

import (
	"fmt"
	"net/http"
)

// Handler returns a ping handler
func Handler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "pong\nazurefuncs version %s\n", version)
	}
}
