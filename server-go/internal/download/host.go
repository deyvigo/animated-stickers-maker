// Package download validates TikTok links and talks to the yt-dlp
// downloader service.
package download

import (
	"net/url"
	"strings"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

// AssertAllowedHost rejects anything that isn't an allowed host *before*
// the URL goes anywhere near yt-dlp. This is the SSRF/abuse boundary:
// without it the endpoint is a generic "download any URL" service.
func AssertAllowedHost(raw string, allowed []string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return apperr.New(models.InvalidURL, "El link no es válido.")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return apperr.New(models.InvalidURL, "El link no es válido.")
	}
	for _, a := range allowed {
		if host == a || strings.HasSuffix(host, "."+a) {
			return nil
		}
	}
	return apperr.New(models.HostNotAllowed, "Solo se admiten links de TikTok.").
		WithDetail("host=" + host)
}
