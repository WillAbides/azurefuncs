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
	exclusive := req.URL.Query().Get("exclusive") != ""
	constraint, err := goversion.NewConstraints(c)
	if err != nil {
		http.Error(w, "invalid constraint", http.StatusBadRequest)
		return
	}
	candidates := req.URL.Query().Get("candidates")
	versions := strings2Versions(strings.Split(candidates, ","))
	result := maxVersionMatch(constraint, versions)

	if !exclusive && result == nil {
		versions, err = h.getVersions()
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		result = maxVersionMatch(constraint, versions)
	}

	if result == nil {
		http.Error(w, "no matching version found", http.StatusNotFound)
		return
	}
	fmt.Fprintln(w, result.String())
}

func strings2Versions(slice []string) []*goversion.Version {
	result := make([]*goversion.Version, 0, len(slice))
	for _, s := range slice {
		v, err := goversion.NewVersion(s)
		if err != nil {
			continue
		}
		result = append(result, v)
	}
	return result
}

func maxVersionMatch(constraints *goversion.Constraints, versions []*goversion.Version) *goversion.Version {
	var result *goversion.Version
	for _, v := range versions {
		if !constraints.Check(v) {
			continue
		}
		if result == nil || v.GreaterThan(result) {
			result = v
		}
	}
	return result
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
