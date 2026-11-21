package xtreamproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/WireshJ/iptv-proxy/pkg/config"
	xtream "github.com/tellytv/go.xtream-codes"
)

// Synthetic fixture mirroring the shapes seen from real providers: empty arrays
// where objects are expected, numbers and strings mixed in the same field, and
// unknown extra fields.
const seriesInfoFixture = `{"seasons":[{"season_number":1,"name":"Season 1","extra_season_field":true}],` +
	`"info":{"name":"Fixture Series","rating":"7.5","unknown_field":{"a":1}},` +
	`"episodes":{"1":[` +
	`{"id":"1","episode_num":1,"title":"Normal","container_extension":"mkv","info":{"duration_secs":1320,"rating":7.4,"audio":{"codec_name":"aac","channels":2},"video":{"codec_name":"h264","width":1280},"bitrate":1500},"season":1,"added":"1700000000","custom_sid":"","direct_source":"","provider_extra":[1,2]},` +
	`{"id":"2","episode_num":2,"title":"Empty info","container_extension":"mp4","info":[],"season":"1","added":"1700000001"},` +
	`{"id":"3","episode_num":3,"title":"Empty media arrays","container_extension":"mp4","info":{"duration_secs":1300,"rating":"7.1","audio":[],"video":[],"bitrate":0},"season":1,"added":"1700000002"}` +
	`]}}`

const vodInfoFixture = `{"info":{"name":"Fixture Movie","rating":"6.5","audio":[],"video":[],"unknown_field":true},"movie_data":{"stream_id":123,"name":"Fixture Movie","added":"1700000000","category_id":"4","container_extension":"mp4","custom_sid":"","direct_source":""}}`

func TestSeriesInfoFixtureBreaksTypedDecoder(t *testing.T) {
	var s xtream.Series
	if err := json.Unmarshal([]byte(seriesInfoFixture), &s); err == nil {
		t.Fatal("expected the typed decoder to reject the fixture")
	}
	var v xtream.VideoOnDemandInfo
	if err := json.Unmarshal([]byte(vodInfoFixture), &v); err == nil {
		t.Fatal("expected the typed VOD decoder to reject the fixture")
	}
}

func TestInfoActionsPreserveProviderJSON(t *testing.T) {
	for _, tc := range []struct{ action, param, body string }{
		{getSerieInfo, "series_id", seriesInfoFixture},
		{getVodInfo, "vod_id", vodInfoFixture},
	} {
		t.Run(tc.action, func(t *testing.T) {
			c := epgTestClient(t, http.StatusOK, tc.body, func(r *http.Request) {
				q := r.URL.Query()
				if r.URL.Host != "provider.invalid" || r.URL.Path != "/player_api.php" || q.Get("action") != tc.action || q.Get(tc.param) != "1045" || q.Get("username") != "fixture-user" || q.Get("password") != "fixture-password" || r.Header.Get("User-Agent") != "fixture-agent" {
					t.Fatalf("incorrect provider request: %s", r.URL.Redacted())
				}
			})
			response, status, err := c.Action(&config.ProxyConfig{}, tc.action, url.Values{tc.param: {"1045"}})
			if err != nil {
				t.Fatalf("status=%d error=%v", status, err)
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != tc.body {
				t.Fatalf("response changed: %s", encoded)
			}
		})
	}
}

func TestInfoActionsErrorsReturnBadGateway(t *testing.T) {
	for _, action := range []struct{ name, param string }{{getSerieInfo, "series_id"}, {getVodInfo, "vod_id"}} {
		for _, tc := range []struct {
			name   string
			status int
			body   string
		}{
			{"empty", 200, ""}, {"html", 200, "<html>failure</html>"}, {"array", 200, `[]`},
			{"truncated", 200, `{"episodes":{"1":[{"id":"1"`}, {"provider-failure", 503, `{"episodes":{}}`},
		} {
			t.Run(action.name+"/"+tc.name, func(t *testing.T) {
				c := epgTestClient(t, tc.status, tc.body, nil)
				_, status, err := c.Action(&config.ProxyConfig{}, action.name, url.Values{action.param: {"1045"}})
				if err == nil || status != http.StatusBadGateway {
					t.Fatalf("status=%d error=%v", status, err)
				}
				for _, secret := range []string{"fixture-password", "fixture-user", "provider.invalid"} {
					if strings.Contains(err.Error(), secret) {
						t.Fatalf("error leaks %q", secret)
					}
				}
			})
		}
	}
}

func sequenceClient(t *testing.T, statuses []int, body string, calls *int) *Client {
	t.Helper()
	c := epgTestClient(t, 200, body, nil)
	c.HTTP = &http.Client{Transport: epgTransport(func(*http.Request) (*http.Response, error) {
		status := statuses[len(statuses)-1]
		if *calls < len(statuses) {
			status = statuses[*calls]
		}
		*calls++
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	return c
}

func TestInfoActionsRetryTransientProviderErrors(t *testing.T) {
	saved := infoRetryDelays
	infoRetryDelays = []time.Duration{0, 0}
	defer func() { infoRetryDelays = saved }()
	for _, tc := range []struct {
		name      string
		statuses  []int
		wantCalls int
		wantOK    bool
	}{
		{"502-then-ok", []int{502, 200}, 2, true},
		{"504-503-ok", []int{504, 503, 200}, 3, true},
		{"always-502", []int{502}, 3, false},
		{"404-not-retried", []int{404, 200}, 1, false},
		{"500-not-retried", []int{500, 200}, 1, false},
	} {
		for _, action := range []struct{ name, param, body string }{{getSerieInfo, "series_id", seriesInfoFixture}, {getVodInfo, "vod_id", vodInfoFixture}} {
			t.Run(action.name+"/"+tc.name, func(t *testing.T) {
				calls := 0
				c := sequenceClient(t, tc.statuses, action.body, &calls)
				_, status, err := c.Action(&config.ProxyConfig{}, action.name, url.Values{action.param: {"1045"}})
				if calls != tc.wantCalls {
					t.Fatalf("calls=%d want %d", calls, tc.wantCalls)
				}
				if tc.wantOK != (err == nil) || (!tc.wantOK && status != http.StatusBadGateway) {
					t.Fatalf("status=%d error=%v", status, err)
				}
			})
		}
	}
}

func TestInfoRetryStopsWhenClientGoesAway(t *testing.T) {
	saved := infoRetryDelays
	infoRetryDelays = []time.Duration{time.Hour, time.Hour}
	defer func() { infoRetryDelays = saved }()
	calls := 0
	c := sequenceClient(t, []int{502}, `{}`, &calls)
	ctx, cancel := context.WithCancel(context.Background())
	c.Context = ctx
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	done := make(chan error, 1)
	go func() { _, _, err := c.Action(&config.ProxyConfig{}, getVodInfo, url.Values{"vod_id": {"1"}}); done <- err }()
	select {
	case err := <-done:
		if err == nil || calls != 1 {
			t.Fatalf("calls=%d error=%v", calls, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry ignored the cancelled request")
	}
}

func TestInfoActionsMissingIDDoNotContactProvider(t *testing.T) {
	for _, action := range []string{getSerieInfo, getVodInfo} {
		t.Run(action, func(t *testing.T) {
			c := epgTestClient(t, 200, `{}`, func(*http.Request) { t.Fatal("unexpected provider request") })
			_, status, err := c.Action(&config.ProxyConfig{}, action, url.Values{})
			if err == nil || status != http.StatusBadRequest {
				t.Fatalf("status=%d error=%v", status, err)
			}
		})
	}
}
