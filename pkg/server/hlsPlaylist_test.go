package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/WireshJ/iptv-proxy/pkg/config"
	"github.com/gin-gonic/gin"
)

const fixturePlaylist = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXT-X-KEY:METHOD=AES-128,URI=\"/hls/tok/101_key.bin\"\n#EXTINF:6.0,\n/hls/tok/101_1.ts\n#EXTINF:6.0,\nhttp://cdn.invalid/hls/tok/101_2.ts\n#EXTINF:6.0,\n/hlsr/tok/provider-user/provider-password/101/h/3.ts\n#EXTINF:6.0,\nhttp://cdn.invalid/live/provider-user/provider-password/4.ts\n"

func hlsFixtureConfig() *Config {
	return &Config{ProxyConfig: &config.ProxyConfig{XtreamBaseURL: "http://provider.invalid", XtreamUser: "provider-user", XtreamPassword: "provider-password", User: "proxy-user", Password: "proxy-password"}}
}

func withHLSFixture(t *testing.T, playlist, segments func(*http.Request) (*http.Response, error)) *gin.Engine {
	t.Helper()
	previousHLS, previousStream := hlsClient, streamClient
	t.Cleanup(func() {
		hlsClient, streamClient = previousHLS, previousStream
		hlsChannelsRedirectURLLock.Lock()
		delete(hlsChannelsRedirectURL, "101.m3u8")
		hlsChannelsRedirectURLLock.Unlock()
	})
	noFollow := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	hlsClient = &http.Client{Transport: epgFixtureTransport(playlist), CheckRedirect: noFollow}
	if segments != nil {
		streamClient = &http.Client{Transport: epgFixtureTransport(segments)}
	}
	router := gin.New()
	hlsFixtureConfig().xtreamRoutes(router.Group("/"))
	return router
}

func fixtureResponse(status int, contentType, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func getPlaylist(router *gin.Engine) *streamRecorder {
	recorder := newStreamRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://proxy.invalid/live/proxy-user/proxy-password/101.m3u8", nil))
	return recorder
}

// F01: a provider answering the live playlist directly with 200 (no 302) must not yield an empty 200.
func TestHLSDirect200KeepsBodyAndSegmentsWork(t *testing.T) {
	router := withHLSFixture(t,
		func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "http://provider.invalid/live/provider-user/provider-password/101.m3u8" {
				t.Fatalf("unexpected playlist request %s", r.URL)
			}
			if r.Header.Get("Accept-Encoding") != "" {
				t.Fatal("client Accept-Encoding forwarded on a rewritten playlist")
			}
			return fixtureResponse(http.StatusOK, "application/vnd.apple.mpegurl", fixturePlaylist, http.Header{"Content-Length": {"999"}}), nil
		},
		func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "provider.invalid" || r.URL.Path != "/hls/tok/101_1.ts" {
				t.Fatalf("unexpected segment request %s", r.URL)
			}
			return fixtureResponse(http.StatusOK, "video/mp2t", "segment-1", nil), nil
		})

	recorder := getPlaylist(router)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.HasPrefix(body, "#EXTM3U") {
		t.Fatalf("status=%d body=%q", recorder.Code, body)
	}
	if !strings.Contains(body, "\n/hls/tok/101_1.ts\n") || !strings.Contains(body, `URI="/hls/tok/101_key.bin"`) {
		t.Fatalf("segment or key URI not routed through the proxy: %q", body)
	}
	if strings.Contains(body, "/hlsr/tok/provider-user/") || !strings.Contains(body, "\n/hlsr/tok/proxy-user/proxy-password/101/h/3.ts\n") {
		t.Fatalf("provider credentials not replaced on a routed URI: %q", body)
	}
	// External URI (other host): left exactly as sent, credentials included.
	if !strings.Contains(body, "\nhttp://cdn.invalid/live/provider-user/provider-password/4.ts\n") {
		t.Fatalf("external URI altered: %q", body)
	}
	if !strings.Contains(body, "http://cdn.invalid/hls/tok/101_2.ts") {
		t.Fatal("URI on another host must be left untouched")
	}
	if got := recorder.Header().Get("Content-Length"); got == "999" {
		t.Fatal("stale upstream Content-Length copied onto the rewritten body")
	}

	segment := newStreamRecorder()
	router.ServeHTTP(segment, httptest.NewRequest(http.MethodGet, "http://proxy.invalid/hls/tok/101_1.ts", nil))
	if segment.Code != http.StatusOK || segment.Body.String() != "segment-1" {
		t.Fatalf("segment status=%d body=%q", segment.Code, segment.Body.String())
	}
}

// 302 (and 307/308) -> 200 on another host: relative URIs resolve against the final URL and
// segments are fetched from that host.
func TestHLSRedirectsToFinalPlaylist(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			router := withHLSFixture(t,
				func(r *http.Request) (*http.Response, error) {
					switch r.URL.Host {
					case "provider.invalid":
						return fixtureResponse(status, "", "", http.Header{"Location": {"http://edge.invalid/hls/tok/index.m3u8"}}), nil
					case "edge.invalid":
						return fixtureResponse(http.StatusOK, "application/x-mpegURL", "#EXTM3U\n#EXTINF:6.0,\n101_7.ts\n", nil), nil
					}
					t.Fatalf("unexpected host %s", r.URL.Host)
					return nil, nil
				},
				func(r *http.Request) (*http.Response, error) {
					if r.URL.Host != "edge.invalid" || r.URL.Path != "/hls/tok/101_7.ts" {
						t.Fatalf("unexpected segment request %s", r.URL)
					}
					return fixtureResponse(http.StatusOK, "video/mp2t", "segment-7", nil), nil
				})
			recorder := getPlaylist(router)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "\n/hls/tok/101_7.ts\n") {
				t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
			}
			segment := newStreamRecorder()
			router.ServeHTTP(segment, httptest.NewRequest(http.MethodGet, "http://proxy.invalid/hls/tok/101_7.ts", nil))
			if segment.Body.String() != "segment-7" {
				t.Fatalf("segment body=%q", segment.Body.String())
			}
		})
	}
}

// 302 -> upstream error must keep the error status, never become a fake 200.
func TestHLSRedirectToErrorKeepsStatus(t *testing.T) {
	router := withHLSFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "provider.invalid" {
			return fixtureResponse(http.StatusFound, "", "", http.Header{"Location": {"http://edge.invalid/hls/tok/index.m3u8"}}), nil
		}
		return fixtureResponse(http.StatusNotFound, "text/plain", "not found", nil), nil
	}, nil)
	recorder := getPlaylist(router)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", recorder.Code)
	}
}

// A redirect loop stops with a gateway error instead of spinning.
func TestHLSRedirectLoopStops(t *testing.T) {
	calls := 0
	router := withHLSFixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return fixtureResponse(http.StatusFound, "", "", http.Header{"Location": {"http://provider.invalid/live/provider-user/provider-password/101.m3u8"}}), nil
	}, nil)
	recorder := getPlaylist(router)
	// hlsMaxRedirects redirects are followed (plus the first request), then the chain is refused.
	if recorder.Code != http.StatusBadGateway || calls != hlsMaxRedirects+1 {
		t.Fatalf("status=%d calls=%d", recorder.Code, calls)
	}
}

func TestRewriteHLSPlaylistRelativeAndCRLF(t *testing.T) {
	c := hlsFixtureConfig()

	// Playlist on the configured provider host: /live/ segments go through the proxy /live route,
	// with the proxy credentials.
	base, _ := url.Parse("http://provider.invalid/live/provider-user/provider-password/index.m3u8?token=x")
	got := rewriteHLSPlaylist("#EXTM3U\r\n#EXTINF:6.0,\r\nseg1.ts?token=x\r\n", base, c.hlsLocalPath(base))
	want := "#EXTM3U\r\n#EXTINF:6.0,\r\n/live/proxy-user/proxy-password/seg1.ts?token=x\r\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// After a redirect to an edge host the /live route would rebuild the URL on XtreamBaseURL
	// (wrong host): those segments stay absolute URLs to the edge, unchanged.
	edge, _ := url.Parse("http://edge.invalid/live/provider-user/provider-password/index.m3u8")
	got = rewriteHLSPlaylist("#EXTM3U\nseg2.ts\n", edge, c.hlsLocalPath(edge))
	want = "#EXTM3U\nhttp://edge.invalid/live/provider-user/provider-password/seg2.ts\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// Paths without a proxy route become absolute provider URLs instead of proxy 404s.
	other, _ := url.Parse("http://edge.invalid/vod/index.m3u8")
	got = rewriteHLSPlaylist("#EXTM3U\nchunk-1.ts\n/other/abs.ts\nhttp://edge.invalid/x/y.ts\n", other, c.hlsLocalPath(other))
	want = "#EXTM3U\nhttp://edge.invalid/vod/chunk-1.ts\nhttp://edge.invalid/other/abs.ts\nhttp://edge.invalid/x/y.ts\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
