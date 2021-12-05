package limit

import (
	"errors"
	"fmt"

	goll "github.com/fabiofenoglio/goll"
	"github.com/gin-gonic/gin"
)

type Config struct {
	Limiter          goll.LoadLimiter
	DefaultRouteLoad uint64
	TenantKey        string
	TenantKeyFunc    func(c *gin.Context) (string, error)
	AcceptHandler    func(c *gin.Context, result goll.SubmitResult)
	AbortHandler     func(c *gin.Context, result goll.SubmitResult)
	ErrorHandler     func(c *gin.Context, err error)
}

type loadLimiterMiddleware struct {
	limiter       goll.LoadLimiter
	load          uint64
	tenantKey     string
	tenantKeyFunc func(c *gin.Context) (string, error)
	acceptHandler func(c *gin.Context, result goll.SubmitResult)
	abortHandler  func(c *gin.Context, result goll.SubmitResult)
	errorHandler  func(c *gin.Context, err error)
}

func NewLimiterMiddleware(config Config) *loadLimiterMiddleware {
	return &loadLimiterMiddleware{
		limiter:       config.Limiter,
		load:          config.DefaultRouteLoad,
		acceptHandler: config.AcceptHandler,
		abortHandler:  config.AbortHandler,
		errorHandler:  config.ErrorHandler,
		tenantKey:     config.TenantKey,
		tenantKeyFunc: config.TenantKeyFunc,
	}
}

func (instance *loadLimiterMiddleware) clone() *loadLimiterMiddleware {
	return &loadLimiterMiddleware{
		limiter:       instance.limiter,
		load:          instance.load,
		acceptHandler: instance.acceptHandler,
		abortHandler:  instance.abortHandler,
		errorHandler:  instance.errorHandler,
		tenantKey:     instance.tenantKey,
		tenantKeyFunc: instance.tenantKeyFunc,
	}
}

func (instance *loadLimiterMiddleware) Default() gin.HandlerFunc {
	return routeLoadLimiter(instance)
}

func (instance *loadLimiterMiddleware) WithLoad(load uint64) gin.HandlerFunc {
	cloned := instance.clone()
	cloned.load = load
	return cloned.Default()
}

func (instance *loadLimiterMiddleware) handleError(c *gin.Context, err error) {
	if instance.errorHandler != nil {
		// run the user error handler if any.
		instance.errorHandler(c, err)
	} else {
		// if no custom handler is present, panic
		panic(fmt.Errorf("error submitting load request: %w", err))
	}
}

func (instance *loadLimiterMiddleware) handleRejection(c *gin.Context, res goll.SubmitResult) {
	if instance.abortHandler != nil {
		// run the user abort handler if any.
		instance.abortHandler(c, res)
	} else {
		// if no custom handler is present, by default
		// we send an HTTP 429 response with X-Retry-In header if available
		if res.RetryInAvailable {
			c.Header("X-Retry-In", fmt.Sprintf("%v", res.RetryIn.Milliseconds()))
		}

		c.AbortWithStatus(429)
	}
}

func (instance *loadLimiterMiddleware) handleAccept(c *gin.Context, res goll.SubmitResult) {
	if instance.acceptHandler != nil {
		// run the user accept handler if any.
		instance.acceptHandler(c, res)
	} else {
		c.Next()
	}
}

func (instance *loadLimiterMiddleware) effectiveTenantKey(c *gin.Context) (string, error) {
	if instance.tenantKeyFunc != nil {
		return instance.tenantKeyFunc(c)
	}
	return instance.tenantKey, nil
}

func routeLoadLimiter(instance *loadLimiterMiddleware) gin.HandlerFunc {
	err := validateConfig(instance)
	if err != nil {
		panic(err)
	}

	return func(c *gin.Context) {
		if instance.load <= 0 {
			// no need to check if route load is zero
			c.Next()
			return
		}

		tenantKey, err := instance.effectiveTenantKey(c)
		// if an error occured, run the error handler.
		// not that a rejected load request is not an error.
		// an error occurs here if the tenant key could not be retrieved
		if err != nil {
			instance.handleError(c, err)
			return
		}

		// submit load request to limiter
		res, err := instance.limiter.Submit(tenantKey, instance.load)

		// if an error occured, run the error handler.
		// not that a rejected load request is not an error.
		// an error only occurs when synchronization is enabled and fails
		// or something like that
		if err != nil {
			instance.handleError(c, err)
			return
		}

		// if the request was rejected we run the abort handler
		if !res.Accepted {
			instance.handleRejection(c, res)
			return
		}

		// the request was accepted so we can go on.
		instance.handleAccept(c, res)
	}
}

func validateConfig(config *loadLimiterMiddleware) error {
	if config == nil {
		return errors.New("nil config")
	}
	if config.limiter == nil {
		return errors.New("limiter is required")
	}
	if config.tenantKey == "" && config.tenantKeyFunc == nil {
		return errors.New("one of TenantKey or TenantKeyFunc is required")
	}
	if config.tenantKey != "" && config.tenantKeyFunc != nil {
		return errors.New("only one of TenantKey or TenantKeyFunc is required")
	}
	return nil
}
