package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withReleaseURL(t *testing.T, url string) {
	t.Helper()
	prev := latestReleaseURL
	latestReleaseURL = url
	t.Cleanup(func() { latestReleaseURL = prev })
}

func TestShowVersion_DegradesOnNetworkError(t *testing.T) {
	// Server that closes the connection so the client errors mid-request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer is not a hijacker")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	withReleaseURL(t, srv.URL)

	got := ShowVersion()
	if !strings.Contains(got, "Current: "+AppVersion) {
		t.Errorf("expected current version line, got %q", got)
	}
	if strings.Contains(got, "Latest:") {
		t.Errorf("expected no Latest line on failure, got %q", got)
	}
}

func TestShowVersion_DegradesOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	withReleaseURL(t, srv.URL)

	got := ShowVersion()
	if strings.Contains(got, "Latest:") {
		t.Errorf("expected no Latest line on 503, got %q", got)
	}
}

func TestShowVersion_DegradesOnBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	withReleaseURL(t, srv.URL)

	got := ShowVersion()
	if strings.Contains(got, "Latest:") {
		t.Errorf("expected no Latest line on bad JSON, got %q", got)
	}
}

func TestShowVersion_ReportsLatestWhenReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"v9.9.9"}`))
	}))
	defer srv.Close()

	withReleaseURL(t, srv.URL)

	got := ShowVersion()
	if !strings.Contains(got, "Latest: v9.9.9") {
		t.Errorf("expected Latest: v9.9.9 in output, got %q", got)
	}
	if !strings.Contains(got, "Please upgrade") {
		t.Errorf("expected upgrade hint when versions differ, got %q", got)
	}
}

func TestShowVersion_NoUpgradeHintWhenCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"` + AppVersion + `"}`))
	}))
	defer srv.Close()

	withReleaseURL(t, srv.URL)

	got := ShowVersion()
	if strings.Contains(got, "Please upgrade") {
		t.Errorf("expected no upgrade hint when current matches latest, got %q", got)
	}
}
