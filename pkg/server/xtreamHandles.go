/*
 * Iptv-Proxy is a project to proxyfie an m3u file and to proxyfie an Xtream iptv service (client API).
 * Copyright (C) 2020  Pierre-Emmanuel Jacquier
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jamesnetherton/m3u"
	xtreamapi "github.com/WireshJ/iptv-proxy/pkg/xtream-proxy"
	"github.com/google/uuid"
)

type cacheMeta struct {
	string
	time.Time
}

var hlsChannelsRedirectURL map[string]url.URL = map[string]url.URL{}
var hlsChannelsRedirectURLLock = sync.RWMutex{}

// XXX Use key/value storage e.g: etcd, redis...
// and remove that dirty globals
var xtreamM3uCache map[string]cacheMeta = map[string]cacheMeta{}
var xtreamM3uCacheLock = sync.RWMutex{}

func (c *Config) cacheXtreamM3u(playlist *m3u.Playlist, cacheName string) error {
	xtreamM3uCacheLock.Lock()
	defer xtreamM3uCacheLock.Unlock()

	tmp := *c
	tmp.playlist = playlist

	path := filepath.Join(os.TempDir(), uuid.New().String()+".iptv-proxy.m3u")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := tmp.marshallInto(f, true); err != nil {
		return err
	}
	xtreamM3uCache[cacheName] = cacheMeta{path, time.Now()}

	return nil
}

func (c *Config) xtreamGenerateM3u(ctx *gin.Context, extension string) (*m3u.Playlist, error) {
	client, err := xtreamapi.New(c.XtreamUser.String(), c.XtreamPassword.String(), c.XtreamBaseURL, ctx.Request.UserAgent())
	if err != nil {
		return nil, err
	}

	cat, err := client.GetLiveCategories()
	if err != nil {
		return nil, err
	}

	// this is specific to xtream API,
	// prefix with "live" if there is an extension.
	var prefix string
	if extension != "" {
		extension = "." + extension
		prefix = "live/"
	}

	var playlist = new(m3u.Playlist)
	playlist.Tracks = make([]m3u.Track, 0)

	for _, category := range cat {
		live, err := client.GetLiveStreams(fmt.Sprint(category.ID))
		if err != nil {
			return nil, err
		}

		for _, stream := range live {
			track := m3u.Track{Name: stream.Name, Length: -1, URI: "", Tags: nil}

			//TODO: Add more tag if needed.
			if stream.EPGChannelID != "" {
				track.Tags = append(track.Tags, m3u.Tag{Name: "tvg-id", Value: stream.EPGChannelID})
			}
			if stream.Name != "" {
				track.Tags = append(track.Tags, m3u.Tag{Name: "tvg-name", Value: stream.Name})
			}
			if stream.Icon != "" {
				track.Tags = append(track.Tags, m3u.Tag{Name: "tvg-logo", Value: stream.Icon})
			}
			if category.Name != "" {
				track.Tags = append(track.Tags, m3u.Tag{Name: "group-title", Value: category.Name})
			}

			track.URI = fmt.Sprintf("%s/%s%s/%s/%s%s", c.XtreamBaseURL, prefix, c.XtreamUser, c.XtreamPassword, fmt.Sprint(stream.ID), extension)
			playlist.Tracks = append(playlist.Tracks, track)
		}
	}

	return playlist, nil
}

func (c *Config) xtreamGetAuto(ctx *gin.Context) {
	newQuery := ctx.Request.URL.Query()
	q := c.RemoteURL.Query()
	for k, v := range q {
		if k == "username" || k == "password" {
			continue
		}

		newQuery.Add(k, strings.Join(v, ","))
	}
	ctx.Request.URL.RawQuery = newQuery.Encode()

	c.xtreamGet(ctx)
}

func (c *Config) xtreamGet(ctx *gin.Context) {
	rawURL := fmt.Sprintf("%s/get.php?username=%s&password=%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword)

	q := ctx.Request.URL.Query()

	for k, v := range q {
		if k == "username" || k == "password" {
			continue
		}

		rawURL = fmt.Sprintf("%s&%s=%s", rawURL, k, strings.Join(v, ","))
	}

	m3uURL, err := url.Parse(rawURL)
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	xtreamM3uCacheLock.RLock()
	meta, ok := xtreamM3uCache[m3uURL.String()]
	d := time.Since(meta.Time)
	if !ok || d.Hours() >= float64(c.M3UCacheExpiration) {
		log.Printf("[iptv-proxy] %v | %s | xtream cache m3u file\n", time.Now().Format("2006/01/02 - 15:04:05"), ctx.ClientIP())
		xtreamM3uCacheLock.RUnlock()
		playlist, err := m3u.Parse(m3uURL.String())
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
			return
		}
		if err := c.cacheXtreamM3u(&playlist, m3uURL.String()); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
			return
		}
	} else {
		xtreamM3uCacheLock.RUnlock()
	}

	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, c.M3UFileName))
	xtreamM3uCacheLock.RLock()
	path := xtreamM3uCache[m3uURL.String()].string
	xtreamM3uCacheLock.RUnlock()
	ctx.Header("Content-Type", "application/octet-stream")

	ctx.File(path)
}

func (c *Config) xtreamApiGet(ctx *gin.Context) {
	const (
		apiGet = "apiget"
	)

	var (
		extension = ctx.Query("output")
		cacheName = apiGet + extension
	)

	xtreamM3uCacheLock.RLock()
	meta, ok := xtreamM3uCache[cacheName]
	d := time.Since(meta.Time)
	if !ok || d.Hours() >= float64(c.M3UCacheExpiration) {
		log.Printf("[iptv-proxy] %v | %s | xtream cache API m3u file\n", time.Now().Format("2006/01/02 - 15:04:05"), ctx.ClientIP())
		xtreamM3uCacheLock.RUnlock()
		playlist, err := c.xtreamGenerateM3u(ctx, extension)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
			return
		}
		if err := c.cacheXtreamM3u(playlist, cacheName); err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
			return
		}
	} else {
		xtreamM3uCacheLock.RUnlock()
	}

	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, c.M3UFileName))
	xtreamM3uCacheLock.RLock()
	path := xtreamM3uCache[cacheName].string
	xtreamM3uCacheLock.RUnlock()
	ctx.Header("Content-Type", "application/octet-stream")

	ctx.File(path)

}

func (c *Config) xtreamPlayerAPIGET(ctx *gin.Context) {
	c.xtreamPlayerAPI(ctx, ctx.Request.URL.Query())
}

func (c *Config) xtreamPlayerAPIPOST(ctx *gin.Context) {
	contents, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	q, err := url.ParseQuery(string(contents))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamPlayerAPI(ctx, q)
}

func (c *Config) xtreamPlayerAPI(ctx *gin.Context, q url.Values) {
	var action string
	if len(q["action"]) > 0 {
		action = q["action"][0]
	}

	client, err := xtreamapi.New(c.XtreamUser.String(), c.XtreamPassword.String(), c.XtreamBaseURL, ctx.Request.UserAgent())
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	resp, httpcode, err := client.Action(c.ProxyConfig, action, q)
	if err != nil {
		ctx.AbortWithError(httpcode, err) // nolint: errcheck
		return
	}

	log.Printf("[iptv-proxy] %v | %s |Action\t%s\n", time.Now().Format("2006/01/02 - 15:04:05"), ctx.ClientIP(), action)

	ctx.JSON(http.StatusOK, resp)
}

func (c *Config) xtreamXMLTV(ctx *gin.Context) {
	client, err := xtreamapi.New(c.XtreamUser.String(), c.XtreamPassword.String(), c.XtreamBaseURL, ctx.Request.UserAgent())
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	resp, err := client.GetXMLTV()
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	ctx.Data(http.StatusOK, "application/xml", resp)
}

func (c *Config) xtreamStreamHandler(ctx *gin.Context) {
	id := ctx.Param("id")
	rpURL, err := url.Parse(fmt.Sprintf("%s/%s/%s/%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword, id))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, rpURL)
}

func (c *Config) xtreamStreamLive(ctx *gin.Context) {
	id := ctx.Param("id")
	rpURL, err := url.Parse(fmt.Sprintf("%s/live/%s/%s/%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword, id))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, rpURL)
}

func (c *Config) xtreamStreamPlay(ctx *gin.Context) {
	token := ctx.Param("token")
	t := ctx.Param("type")
	rpURL, err := url.Parse(fmt.Sprintf("%s/play/%s/%s", c.XtreamBaseURL, token, t))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, rpURL)
}

func (c *Config) xtreamStreamTimeshift(ctx *gin.Context) {
	duration := ctx.Param("duration")
	start := normalizeTimeshiftStart(ctx.Param("start"))
	id := ctx.Param("id")
	rpURL, err := url.Parse(fmt.Sprintf("%s/timeshift/%s/%s/%s/%s/%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword, duration, start, id))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.stream(ctx, rpURL)
}

func (c *Config) xtreamStreamMovie(ctx *gin.Context) {
	id := ctx.Param("id")
	rpURL, err := url.Parse(fmt.Sprintf("%s/movie/%s/%s/%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword, id))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, rpURL)
}

func (c *Config) xtreamStreamSeries(ctx *gin.Context) {
	id := ctx.Param("id")
	rpURL, err := url.Parse(fmt.Sprintf("%s/series/%s/%s/%s", c.XtreamBaseURL, c.XtreamUser, c.XtreamPassword, id))
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, rpURL)
}

func (c *Config) xtreamHlsStream(ctx *gin.Context) {
	chunk := ctx.Param("chunk")
	s := strings.Split(chunk, "_")
	if len(s) != 2 {
		ctx.AbortWithError( // nolint: errcheck
			http.StatusInternalServerError,
			errors.New("HSL malformed chunk"),
		)
		return
	}
	channel := s[0]

	url, err := getHlsRedirectURL(channel)
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	req, err := url.Parse(
		fmt.Sprintf(
			"%s://%s/hls/%s/%s",
			url.Scheme,
			url.Host,
			ctx.Param("token"),
			ctx.Param("chunk"),
		),
	)

	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, req)
}

func (c *Config) xtreamHlsrStream(ctx *gin.Context) {
	channel := ctx.Param("channel")

	url, err := getHlsRedirectURL(channel)
	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	req, err := url.Parse(
		fmt.Sprintf(
			"%s://%s/hlsr/%s/%s/%s/%s/%s/%s",
			url.Scheme,
			url.Host,
			ctx.Param("token"),
			c.XtreamUser,
			c.XtreamPassword,
			ctx.Param("channel"),
			ctx.Param("hash"),
			ctx.Param("chunk"),
		),
	)

	if err != nil {
		ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
		return
	}

	c.xtreamStream(ctx, req)
}

func getHlsRedirectURL(channel string) (*url.URL, error) {
	hlsChannelsRedirectURLLock.RLock()
	defer hlsChannelsRedirectURLLock.RUnlock()

	url, ok := hlsChannelsRedirectURL[channel+".m3u8"]
	if !ok {
		return nil, errors.New("HSL redirect url not found")
	}

	return &url, nil
}

var hlsClient = &http.Client{
	Transport: transport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// hlsMaxRedirects bounds the redirect chain followed for a live HLS playlist.
const hlsMaxRedirects = 5

// hlsMaxPlaylistBytes bounds the playlist body read into memory before rewriting.
const hlsMaxPlaylistBytes = 8 << 20

var hlsURIAttribute = regexp.MustCompile(`URI="([^"]*)"`)

func isHLSRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// rewriteHLSPlaylist resolves every URI against the final playlist URL. When local returns a
// proxy path for it, that host-relative path is written (the client fetches it through the proxy);
// every other URI is written as an absolute URL to its original host, untouched otherwise.
func rewriteHLSPlaylist(body string, base *url.URL, local func(*url.URL) (string, bool)) string {
	rewrite := func(raw string) string {
		ref, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return raw
		}
		resolved := base.ResolveReference(ref)
		if path, ok := local(resolved); ok {
			return path
		}
		if ref.IsAbs() {
			return raw
		}
		return resolved.String()
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		suffix := line[len(trimmed):]
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "#"):
			lines[i] = hlsURIAttribute.ReplaceAllStringFunc(trimmed, func(m string) string {
				inner := hlsURIAttribute.FindStringSubmatch(m)[1]
				return `URI="` + rewrite(inner) + `"`
			}) + suffix
		default:
			lines[i] = rewrite(trimmed) + suffix
		}
	}
	return strings.Join(lines, "\n")
}

// hlsLocalPath maps a provider URI of a playlist served from final to the proxy route that
// serves it, with the provider credentials replaced by the proxy ones:
//   - /hls/:token/<channel>_<n> and /hlsr/... on the playlist host (segment routes resolve the
//     host from the stored playlist URL of the channel);
//   - /live/<provider user>/<provider password>/... only on the configured provider host,
//     because the /live route always rebuilds the upstream URL from XtreamBaseURL.
// Other URIs (other hosts, unknown paths) are not routed and keep their original form.
func (c *Config) hlsLocalPath(final *url.URL) func(*url.URL) (string, bool) {
	providerCreds := "/" + c.XtreamUser.String() + "/" + c.XtreamPassword.String() + "/"
	proxyCreds := "/" + c.User.String() + "/" + c.Password.String() + "/"
	providerHost := ""
	if base, err := url.Parse(c.XtreamBaseURL); err == nil {
		providerHost = base.Host
	}
	return func(u *url.URL) (string, bool) {
		if !strings.EqualFold(u.Host, final.Host) {
			return "", false
		}
		p := u.Path
		switch {
		case strings.HasPrefix(p, "/hlsr/"):
		case strings.HasPrefix(p, "/hls/"):
			parts := strings.Split(strings.TrimPrefix(p, "/hls/"), "/")
			if len(parts) != 2 || !strings.Contains(parts[1], "_") {
				return "", false
			}
		case strings.HasPrefix(p, "/live"+providerCreds) && strings.EqualFold(u.Host, providerHost):
		default:
			return "", false
		}
		return strings.Replace(u.RequestURI(), providerCreds, proxyCreds, 1), true
	}
}

func (c *Config) hlsXtreamStream(ctx *gin.Context, oriURL *url.URL) {
	id := ctx.Param("id")
	current := oriURL
	var resp *http.Response
	for hop := 0; ; hop++ {
		req, err := http.NewRequestWithContext(ctx.Request.Context(), "GET", current.String(), nil)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err) // nolint: errcheck
			return
		}
		mergeHttpHeader(req.Header, ctx.Request.Header)
		// The playlist body is rewritten: let the transport negotiate and decode compression itself.
		req.Header.Del("Accept-Encoding")
		resp, err = hlsClient.Do(req)
		if err != nil {
			ctx.AbortWithError(http.StatusBadGateway, err) // nolint: errcheck
			return
		}
		if !isHLSRedirect(resp.StatusCode) {
			break
		}
		location, err := resp.Location()
		resp.Body.Close()
		if err != nil {
			ctx.AbortWithError(http.StatusBadGateway, err) // nolint: errcheck
			return
		}
		if hop >= hlsMaxRedirects {
			ctx.AbortWithError(http.StatusBadGateway, errors.New("too many HLS redirects")) // nolint: errcheck
			return
		}
		current = location
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, hlsMaxPlaylistBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		ctx.AbortWithError(http.StatusBadGateway, err) // nolint: errcheck
		return
	}
	if len(b) > hlsMaxPlaylistBytes {
		ctx.AbortWithError(http.StatusBadGateway, errors.New("HLS playlist too large")) // nolint: errcheck
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Upstream error (also after a redirect): keep its status, never turn it into a fake 200.
		ctx.Data(resp.StatusCode, contentType, b)
		return
	}

	// Segment routes (/hls, /hlsr) resolve the provider host from the playlist URL of this channel.
	hlsChannelsRedirectURLLock.Lock()
	hlsChannelsRedirectURL[id] = *current
	hlsChannelsRedirectURLLock.Unlock()

	// Credentials are swapped only in URIs routed through the proxy; external URIs stay as sent.
	body := rewriteHLSPlaylist(string(b), current, c.hlsLocalPath(current))

	header := resp.Header.Clone()
	for _, name := range []string{"Content-Length", "Content-Encoding", "Transfer-Encoding", "Content-Type"} {
		header.Del(name)
	}
	mergeHttpHeader(ctx.Writer.Header(), header)
	if contentType == "" {
		contentType = "application/vnd.apple.mpegurl"
	}
	ctx.Data(resp.StatusCode, contentType, []byte(body))
}
