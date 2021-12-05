# gin-goll

[![Documentation](https://godoc.org/github.com/fabiofenoglio/gin-goll?status.svg)](http://godoc.org/github.com/fabiofenoglio/gin-goll)
[![Go Report Card](https://goreportcard.com/badge/github.com/fabiofenoglio/gin-goll)](https://goreportcard.com/report/github.com/fabiofenoglio/gin-goll)

gin-goll is the middleware plugin for integrating a [Goll](https://github.com/fabiofenoglio/goll) load limiter in a Gin-gonic application.

- [Installation](#installation)
- [Quickstart](#quickstart)
- [Full example](#full-example)

## Installation

```
go get github.com/fabiofenoglio/goll
go get github.com/fabiofenoglio/gin-goll
```

## Full example

```go
package main

import (
	"fmt"
	"time"

	gingoll "github.com/fabiofenoglio/gin-goll"
	goll "github.com/fabiofenoglio/goll"
	"github.com/gin-gonic/gin"
)

func main() {

	limiter, _ := goll.New(&goll.Config{
		MaxLoad:           100,
		WindowSize:        10 * time.Second,
	})

	// instantiate the limiter middleware using the limiter instance just created.
	// we assign a default load of "1" per route
	ginLimiter := gingoll.NewLimiterMiddleware(gingoll.Config{
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
				c.String(429, fmt.Sprintf("Too much! retry in %v ms", result.RetryIn.Milliseconds()))
				c.Abort()
			} else {
				c.AbortWithStatus(429)
			}
		},
	})

	r := gin.Default()
	
	// plugin the load limiter middleware for all routes like this:
	r.Use(ginLimiter.Default())

	// or on single route
	r.GET("/something", ginLimiter.Default(), routeHandler)

	// specify per-route load
	r.POST("/create-something", ginLimiter.WithLoad(5), routeHandler)
	r.PUT("/update-something", ginLimiter.WithLoad(3), routeHandler)

	// on route group with specific load
	r.Group("/intensive-operations/").Use(ginLimiter.WithLoad(10))

	// ...

	err := r.Run(":9000")
	if err != nil {
		panic(err)
	}
}

```