package main

import (
	"fmt"
	"net/http"
	"time"
)

// newHTTPServer builds the listener with the timeouts a bare &http.Server
// leaves at zero. ReadTimeout and WriteTimeout stay unset on purpose:
// media uploads and CSV exports legitimately run long, and a blanket cap
// would truncate them.
func newHTTPServer(port int, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
