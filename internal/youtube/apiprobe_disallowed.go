//go:build youtube_apiprobe

// Built only with `-tags youtube_apiprobe`, which no production path uses. The point is the pair of tests in
// youtube_guard_test.go: this file proves the detector has something real to find, so the guard cannot pass
// by silently matching nothing. It is never compiled into the server.

package youtube

import "net/http"

// ProbeClient is deliberately the thing the package must never contain.
func ProbeClient() *http.Client { return http.DefaultClient }
