package xtreamproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Providers return empty arrays where the typed Xtream decoder expects objects
// (episode info, audio, video) and mix numbers and strings in the same field.
// The typed decoder rejects such documents and the proxy answered 502, so the
// provider's JSON is forwarded unchanged instead.
func (c *Client) seriesInfoResponse(seriesID string) (json.RawMessage, error) {
	return c.rawInfoResponse("get_series_info", "series_id", seriesID, "series info")
}

func (c *Client) vodInfoResponse(vodID string) (json.RawMessage, error) {
	return c.rawInfoResponse("get_vod_info", "vod_id", vodID, "VOD info")
}

func (c *Client) rawInfoResponse(action, idParam, id, label string) (json.RawMessage, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + "/player_api.php")
	if err != nil {
		return nil, errors.New("invalid " + label + " provider address")
	}
	q := u.Query()
	q.Set("username", c.Username)
	q.Set("password", c.Password)
	q.Set("action", action)
	q.Set(idParam, id)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(c.Context, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("could not build " + label + " request")
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.doWithTransientRetry(req)
	if err != nil {
		return nil, errors.New(label + " provider request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(label + " provider returned an unsuccessful response")
	}
	const maxInfoBytes = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInfoBytes+1))
	if err != nil || len(body) > maxInfoBytes {
		return nil, errors.New("could not read " + label + " provider response")
	}
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return nil, errors.New(label + " provider returned invalid JSON")
	}
	if len(body) == 0 || body[0] != '{' {
		return nil, errors.New(label + " provider response is not a JSON object")
	}
	return json.RawMessage(body), nil
}

// infoRetryDelays are the pauses before the second and third attempt. Some
// providers answer 502/503/504 intermittently on single titles and succeed a
// moment later; other statuses and transport errors are not retried.
var infoRetryDelays = []time.Duration{700 * time.Millisecond, 1500 * time.Millisecond}

func transientStatus(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}

func (c *Client) doWithTransientRetry(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.HTTP.Do(req)
		if err != nil || !transientStatus(resp.StatusCode) || attempt >= len(infoRetryDelays) {
			return resp, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(infoRetryDelays[attempt]):
		}
	}
}
