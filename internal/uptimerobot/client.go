// Package uptimerobot is a minimal client for the UptimeRobot v3 API.
//
// All requests pass a client-side rate limiter. Lists are cached per path for the
// configured TTL, so reconciling n resources costs one list request per TTL instead of n.
package uptimerobot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const DefaultURL = "https://api.uptimerobot.com/v3"

// Object is a generic API object as returned by UptimeRobot.
type Object map[string]any

func (o Object) ID() int64 {
	f, _ := o["id"].(float64)
	return int64(f)
}

func (o Object) String(key string) string {
	s, _ := o[key].(string)
	return s
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("uptimerobot: %d %s", e.StatusCode, e.Message)
}

func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

type Client struct {
	url     string
	apiKey  string
	http    *http.Client
	limiter *rate.Limiter
	ttl     time.Duration
	// pausedUntil (unix nanoseconds) delays all requests after a 429 response.
	pausedUntil atomic.Int64

	mu    sync.Mutex
	cache map[string]*list
}

type list struct {
	fetched time.Time
	items   map[int64]Object
}

// New returns a client allowing at most requestsPerMinute API calls and caching lists for ttl.
func New(url, apiKey string, requestsPerMinute int, ttl time.Duration) *Client {
	return &Client{
		url:     url,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(float64(requestsPerMinute)/60), 1),
		ttl:     ttl,
		cache:   map[string]*list{},
	}
}

// Hash returns a keyed hash of v. Keying with the API key prevents brute-forcing secrets contained in v.
func (c *Client) Hash(v any) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, []byte(c.apiKey))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Get returns the object with the given id from the cached list at path, or nil if it does not exist.
func (c *Client) Get(ctx context.Context, path string, id int64) (Object, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := c.cache[path]
	if l == nil || time.Since(l.fetched) > c.ttl {
		items := map[int64]Object{}
		for next := c.url + path; next != ""; {
			var page struct {
				Data     []Object `json:"data"`
				NextLink *string  `json:"nextLink"`
			}
			if err := c.do(ctx, http.MethodGet, next, nil, &page); err != nil {
				return nil, err
			}
			for _, o := range page.Data {
				items[o.ID()] = o
			}
			next = ""
			if page.NextLink != nil {
				next = *page.NextLink
			}
		}
		l = &list{fetched: time.Now(), items: items}
		c.cache[path] = l
	}
	if o, ok := l.items[id]; ok {
		return maps.Clone(o), nil
	}
	return nil, nil
}

func (c *Client) Create(ctx context.Context, path string, body any) (Object, error) {
	return c.mutate(ctx, http.MethodPost, path, 0, "", body)
}

func (c *Client) Update(ctx context.Context, path string, id int64, body any) (Object, error) {
	return c.mutate(ctx, http.MethodPatch, path, id, "", body)
}

// Action calls POST path/id/action, e.g. /monitors/1/pause.
func (c *Client) Action(ctx context.Context, path string, id int64, action string) (Object, error) {
	return c.mutate(ctx, http.MethodPost, path, id, "/"+action, nil)
}

func (c *Client) Delete(ctx context.Context, path string, id int64) error {
	_, err := c.mutate(ctx, http.MethodDelete, path, id, "", nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) mutate(ctx context.Context, method, path string, id int64, suffix string, body any) (Object, error) {
	url := c.url + path
	if id != 0 {
		url += "/" + strconv.FormatInt(id, 10) + suffix
	}
	var o Object
	err := c.do(ctx, method, url, body, &o)

	c.mu.Lock()
	defer c.mu.Unlock()
	if l := c.cache[path]; l != nil {
		switch {
		case IsNotFound(err), method == http.MethodDelete && err == nil:
			delete(l.items, id)
		case err != nil:
		case o.ID() != 0:
			l.items[o.ID()] = o
		default:
			delete(c.cache, path)
		}
	}
	if err != nil {
		return nil, err
	}
	return maps.Clone(o), nil
}

func (c *Client) do(ctx context.Context, method, url string, in, out any) error {
	if d := time.Until(time.Unix(0, c.pausedUntil.Load())); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	log.FromContext(ctx).V(1).Info("API request", "method", method, "url", url, "status", resp.StatusCode,
		"rateLimitRemaining", resp.Header.Get("X-RateLimit-Remaining"))
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		e := &APIError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
		var msg struct {
			Message any `json:"message"`
		}
		if json.Unmarshal(data, &msg) == nil && msg.Message != nil {
			e.Message = fmt.Sprint(msg.Message)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := time.Minute
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				retryAfter = time.Duration(s) * time.Second
			}
			c.pausedUntil.Store(time.Now().Add(retryAfter).UnixNano())
		}
		return e
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
