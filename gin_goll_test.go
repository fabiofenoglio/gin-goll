package limit

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"testing"
	"time"

	goll "github.com/fabiofenoglio/goll"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGinWithLoadLimiter(t *testing.T) {
	r := gin.Default()

	limiter, _ := goll.New(&goll.Config{
		MaxLoad:    100,
		WindowSize: 3 * time.Second,
	})

	// instantiate the limiter middleware using the limiter instance just created.
	// we assign a default load of "1" per route
	ginLimiter := NewLimiterMiddleware(Config{
		// Limiter is the goll.LoadLimiter instance
		Limiter: limiter,

		// DefaultRouteLoad is the default load per each route
		// when no route-specific configuration is available
		DefaultRouteLoad: 1,

		// TenantKeyFunc extracts the tenant key from the request context.
		//
		// For instance you can return the request origin IP
		// if you want to limit the load on a per-IP basis,
		// or you could return the username/id of an authenticated client.
		//
		// If you have a single tenant or want to limit globally
		// you can return a fixed string or use the TenantKey parameter instead.
		TenantKeyFunc: func(c *gin.Context) (string, error) {
			// we will limit the load on a per-ip basis
			return c.ClientIP(), nil
		},

		// AbortHandler decides how we respond when a request
		// exceeds the load limit
		AbortHandler: func(c *gin.Context, result goll.SubmitResult) {
			if result.RetryInAvailable {
				c.Header("X-Retry-In", fmt.Sprintf("%v", result.RetryIn.Milliseconds()))
			}
			c.AbortWithStatus(429)
		},

		// ErrorHandler is optional
		ErrorHandler: func(c *gin.Context, err error) {
			c.AbortWithStatusJSON(500, err.Error())
		},
	})

	// plugin the load limiter middleware for all routes like this:
	// r.Use(ginLimiter.Default())

	// and/or on single routes, with route-specific configuration, like this:
	// r.GET("/", ginLimiter.WithLoad(10), routeHandler)
	r.GET("/", ginLimiter.WithLoad(10), func(c *gin.Context) {})

	go func() {
		err := r.Run(":9000")
		if err != nil {
			t.Error("error running the test http server", err.Error())
		}
	}()

	runtime.Gosched()

	// let's run a series of requests and check that
	// the limiter breaks and restores in the expected way.
	for i := 0; i < 15; i++ {
		resp, err := http.DefaultClient.Get("http://127.0.0.1:9000")
		if err != nil {
			t.Error("unexpected error in http request", err.Error())
			return
		}

		switch {
		case i < 10:
			if resp.StatusCode != 200 {
				t.Errorf("unexpected status code %v", resp.StatusCode)
			}
		case i == 10:
			if resp.StatusCode != 429 {
				t.Errorf("expected 429, got %v", resp.StatusCode)
			} else {
				// if we call from another ip we are allowed
				// because we are using the IP as tenant key
				req2, _ := http.NewRequest("GET", "http://127.0.0.1:9000", nil)
				req2.RemoteAddr = "127.0.0.15"
				req2.Header.Add("X-Forwarded-For", req2.RemoteAddr)
				resp2, _ := http.DefaultClient.Do(req2)
				assert.Equal(t, 200, resp2.StatusCode)

				// check the retry-in header
				retryHeader := resp.Header["X-Retry-In"]
				parsed, _ := strconv.Atoi(retryHeader[0])
				assert.Greater(t, parsed, 0)

				// wait the required amount of time
				t.Logf("got retry-in header \"%s\" asking for %v ms", retryHeader[0], parsed)
				time.Sleep(time.Duration(parsed) * time.Millisecond)
			}
		case i > 10:
			if resp.StatusCode == 429 {
				t.Error("unexpected 429 after waiting")
			}
		}
	}
}
