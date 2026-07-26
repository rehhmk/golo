// Package sportmonks implements the providers.Provider interface against the
// SportMonks Football API v3 (https://docs.sportmonks.com/v3/).
//
// Authentication is a query-string api_token (SportMonks v3 has no header-based
// auth). The livescores/inplay endpoint already scopes results to matches
// currently in progress, so ListLiveMatches does not need to interpret
// fixture.state_id itself.
package sportmonks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultTimeout = 8 * time.Second

// errRateLimited marks an HTTP 429 from SportMonks. The quota is hourly, so
// continuing to poll through it only burns requests that could not have
// succeeded — the provider backs off on this error instead of retrying.
var errRateLimited = errors.New("sportmonks: rate limit reached")

type client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newClient(apiKey, baseURL string) *client {
	return &client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// apiEnvelope mirrors the {"data": ...} wrapper SportMonks uses on every response.
type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

func (c *client) get(ctx context.Context, path string, query url.Values) (json.RawMessage, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("api_token", c.apiKey)

	reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, path, query.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sportmonks: building request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sportmonks: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sportmonks: reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sportmonks: unexpected status %d: %s", resp.StatusCode, truncate(body, 500))
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("sportmonks: decoding envelope: %w", err)
	}

	return envelope.Data, nil
}

func unmarshalArray(data json.RawMessage, out interface{}) error {
	return json.Unmarshal(data, out)
}

func unmarshalObject(data json.RawMessage, out interface{}) error {
	return json.Unmarshal(data, out)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
