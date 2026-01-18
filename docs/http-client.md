# HTTP Client

The `core.Client` handles all HTTP communication with retry logic and optional rate limiting.

## Default Configuration

```go
client := registries.DefaultClient()
```

Creates a client with:
- 30 second timeout
- 5 retry attempts
- Exponential backoff starting at 50ms
- Retry on 429 (rate limit) and 5xx (server error) responses

## Client Structure

```go
type Client struct {
    HTTPClient  *http.Client
    MaxRetries  int
    BaseDelay   time.Duration
    RateLimiter RateLimiter
}
```

## Methods

### GetJSON

Fetches JSON and unmarshals into the provided struct:

```go
var resp packageResponse
err := client.GetJSON(ctx, "https://api.example.com/pkg/foo", &resp)
```

Sets `Accept: application/json` header automatically.

### GetBody

Fetches raw response body as bytes:

```go
body, err := client.GetBody(ctx, "https://example.com/DESCRIPTION")
// body is []byte
```

Useful for non-JSON APIs (CRAN, Hackage, Julia).

## Retry Logic

```go
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
    var lastErr error

    for attempt := 0; attempt <= c.MaxRetries; attempt++ {
        if attempt > 0 {
            delay := c.BaseDelay * time.Duration(1<<uint(attempt-1))  // Exponential backoff
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
            }
        }

        if c.RateLimiter != nil {
            if err := c.RateLimiter.Wait(ctx); err != nil {
                return nil, err
            }
        }

        resp, err := c.HTTPClient.Do(req)
        if err != nil {
            lastErr = err
            continue
        }

        if resp.StatusCode == 429 || resp.StatusCode >= 500 {
            resp.Body.Close()
            lastErr = &HTTPError{StatusCode: resp.StatusCode}
            continue
        }

        return resp, nil
    }

    return nil, lastErr
}
```

Backoff sequence: 50ms, 100ms, 200ms, 400ms, 800ms

## Rate Limiting

Implement the `RateLimiter` interface:

```go
type RateLimiter interface {
    Wait(ctx context.Context) error
}
```

Example with `golang.org/x/time/rate`:

```go
import "golang.org/x/time/rate"

type limiter struct {
    l *rate.Limiter
}

func (l *limiter) Wait(ctx context.Context) error {
    return l.l.Wait(ctx)
}

client := registries.DefaultClient()
client.RateLimiter = &limiter{rate.NewLimiter(10, 1)}  // 10 requests/second
```

## Custom HTTP Client

For authentication or custom transports:

```go
httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &authTransport{
        Token: os.Getenv("REGISTRY_TOKEN"),
        Base:  http.DefaultTransport,
    },
}

client := &registries.Client{
    HTTPClient: httpClient,
    MaxRetries: 3,
    BaseDelay:  100 * time.Millisecond,
}

reg, _ := registries.New("npm", "https://npm.pkg.github.com", client)
```

Auth transport example:

```go
type authTransport struct {
    Token string
    Base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("Authorization", "Bearer "+t.Token)
    return t.Base.RoundTrip(req)
}
```

## Error Handling

The client wraps HTTP errors:

```go
type HTTPError struct {
    StatusCode int
    Status     string
    Body       string
}

func (e *HTTPError) Error() string {
    return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Status)
}

func (e *HTTPError) IsNotFound() bool {
    return e.StatusCode == 404
}
```

Usage in registry implementations:

```go
if err := r.client.GetJSON(ctx, url, &resp); err != nil {
    if httpErr, ok := err.(*core.HTTPError); ok && httpErr.IsNotFound() {
        return nil, &core.NotFoundError{Ecosystem: ecosystem, Name: name}
    }
    return nil, err
}
```

## Context Support

All methods accept a context for cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

pkg, err := reg.FetchPackage(ctx, "serde")
if err == context.DeadlineExceeded {
    // Request timed out
}
```

## Performance Characteristics

From benchmarks (mock server, Apple M1 Pro):

| Operation | Time | Memory | Allocs |
|-----------|------|--------|--------|
| GetJSON | 57 µs | 9.5 KB | 126 |
| GetBody | 52 µs | 7.3 KB | 79 |

Most time is spent in:
- syscall (HTTP round-trip): ~36%
- Thread synchronization: ~19%
- I/O polling: ~18%
- JSON parsing: <3%

Memory allocation breakdown:
- HTTP headers: ~60 MB (8.5%)
- MIME parsing: ~59 MB (8.4%)
- io.ReadAll: ~53 MB (7.6%)
- JSON encoding: ~49 MB (7%)
