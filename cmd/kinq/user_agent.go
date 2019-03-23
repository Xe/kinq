package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

func init() {
	http.DefaultTransport = &userAgentTransport{http.DefaultTransport}
}

func genUserAgent() string {
	return fmt.Sprintf("kinq/dev (%s/%s/%s; %s/bot; +https://kinq.cetacean.club/info) Run-By/Cadey (@Cadey~#1337; +https://t.me/miamorecadenza)", runtime.Version(), runtime.GOOS, runtime.GOARCH, filepath.Base(os.Args[0]))
}

type userAgentTransport struct {
	rt http.RoundTripper
}

func (uat userAgentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("User-Agent", genUserAgent())
	return uat.rt.RoundTrip(r)
}
