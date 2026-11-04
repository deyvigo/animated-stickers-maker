package download

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/config"
	"stickers-server/internal/models"
)

var hosts = config.Default().AllowedHosts

func TestAllowedHosts(t *testing.T) {
	for _, u := range []string{
		"https://www.tiktok.com/@user/video/123",
		"https://vm.tiktok.com/abc123",
		"https://vt.tiktok.com/abc123",
		"https://m.tiktok.com/v/123.html",
		"HTTPS://WWW.TIKTOK.COM/x",
	} {
		if err := AssertAllowedHost(u, hosts); err != nil {
			t.Errorf("%s rejected: %v", u, err)
		}
	}
}

func TestDisallowedHosts(t *testing.T) {
	for _, u := range []string{
		"https://youtube.com/watch?v=abc",
		"https://evil.com/tiktok.com",
		"https://tiktok.com.evil.com/video",
		"https://tiktok.com@evil.com/video",
		"https://eviltiktok.com/video",
		"ftp://tiktok.com/video",
		"not a url",
		"",
	} {
		err := AssertAllowedHost(u, hosts)
		var ae *apperr.Error
		if !errors.As(err, &ae) || (ae.Code != models.HostNotAllowed && ae.Code != models.InvalidURL) {
			t.Errorf("%q: err = %v", u, err)
		}
	}
}

func TestClientSuccessAndErrorMapping(t *testing.T) {
	var gotBody map[string]string
	status := http.StatusOK
	reply := `{}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL+"/", 5*time.Second)

	if err := c.Download(context.Background(), "job1", "https://vm.tiktok.com/x"); err != nil {
		t.Fatal(err)
	}
	if gotBody["job_id"] != "job1" || gotBody["url"] != "https://vm.tiktok.com/x" {
		t.Fatalf("body = %v", gotBody)
	}

	status, reply = 400, `{"code":"video_private","message":"Este video es privado.","detail":"d"}`
	var ae *apperr.Error
	err := c.Download(context.Background(), "job1", "u")
	if !errors.As(err, &ae) || ae.Code != models.VideoPrivate || ae.Detail != "d" {
		t.Fatalf("err = %v", err)
	}

	status, reply = 502, `<html>bad gateway</html>`
	err = c.Download(context.Background(), "job1", "u")
	if !errors.As(err, &ae) || ae.Code != models.DownloadFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestClientUnreachableAndTimeout(t *testing.T) {
	var ae *apperr.Error

	err := NewHTTPClient("http://127.0.0.1:1", time.Second).Download(context.Background(), "j", "u")
	if !errors.As(err, &ae) || ae.Code != models.DownloadFailed {
		t.Fatalf("unreachable: %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	err = NewHTTPClient(slow.URL, 50*time.Millisecond).Download(context.Background(), "j", "u")
	if !errors.As(err, &ae) || ae.Code != models.Timeout {
		t.Fatalf("timeout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = NewHTTPClient(slow.URL, time.Second).Download(ctx, "j", "u")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
