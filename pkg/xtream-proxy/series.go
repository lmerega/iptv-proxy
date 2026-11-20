package xtreamproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	resp, err := c.HTTP.Do(req)
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
