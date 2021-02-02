package goversionselect

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/willabides/goversions/goversion"
)

// Handler is an http handler
type Handler struct {
	VersionsMaxAge time.Duration
	VersionsSource string
	versionsMux    sync.Mutex
	versionsTime   time.Time
	versions       []*goversion.Version
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := req.URL.Query().Get("constraint")
	if c == "" {
		c = "1.x"
	}
	constraint, err := goversion.NewConstraints(c)
	if err != nil {
		http.Error(w, "invalid constraint", http.StatusBadRequest)
		return
	}
	candidates := req.URL.Query().Get("candidates")
	var versions []*goversion.Version
	if candidates != "" {
		for _, s := range strings.Split(candidates, ",") {
			v, err := goversion.NewVersion(s)
			if err != nil {
				http.Error(w,
					fmt.Sprintf("invalid go version %q", s),
					http.StatusBadRequest)
			}
			versions = append(versions, v)
		}
	} else {
		versions, err = h.getVersions()
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
	var result *goversion.Version
	for _, v := range versions {
		if !constraint.Check(v) {
			continue
		}
		if result == nil || v.GreaterThan(result) {
			result = v
		}
	}
	if result == nil {
		http.Error(w, "no matching version found", http.StatusNotFound)
		return
	}
	fmt.Fprintln(w, result.String())
}

func (h *Handler) getVersions() ([]*goversion.Version, error) {
	h.versionsMux.Lock()
	defer h.versionsMux.Unlock()
	needsRefresh := false
	if h.versionsTime.IsZero() {
		needsRefresh = true
	}
	if time.Since(h.versionsTime) > h.VersionsMaxAge {
		needsRefresh = true
	}
	if !needsRefresh {
		return h.versions, nil
	}
	resp, err := http.Get(h.VersionsSource)
	if err != nil {
		return h.versions, err
	}
	if resp.StatusCode != 200 {
		return h.versions, fmt.Errorf("not OK")
	}
	versions := make([]*goversion.Version, 0, len(h.versions))
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		if l == "" {
			continue
		}
		var v *goversion.Version
		v, err = goversion.NewVersion(l)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	err = scanner.Err()
	if err != nil {
		return h.versions, err
	}
	h.versions = versions
	h.versionsTime = time.Now()
	return h.versions, resp.Body.Close()
}
