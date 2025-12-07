package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// RateLimiter manages rate limits per IP
type RateLimiter struct {
	ips map[string]*rate.Limiter
	mu  *sync.RWMutex
	r   rate.Limit
	b   int
}

func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		ips: make(map[string]*rate.Limiter),
		mu:  &sync.RWMutex{},
		r:   r,
		b:   b,
	}
}

func (i *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}

	return limiter
}

// RouterConfig holds the dependencies for the router
type RouterConfig struct {
	TaskHandler *TaskHandler
	Logger      zerolog.Logger
}

func SetupRouter(cfg RouterConfig) *gin.Engine {
	// Set Gin mode based on env, defaulting to release for performance if not specified
	// gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// 1. Recovery Middleware
	r.Use(gin.Recovery())

	// 2. Custom Logger Middleware with Zerolog
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		event := cfg.Logger.Info()
		if c.Writer.Status() >= 500 {
			event = cfg.Logger.Error()
		} else if c.Writer.Status() >= 400 {
			event = cfg.Logger.Warn()
		}

		event.
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Str("ip", c.ClientIP()).
			Dur("duration", duration).
			Msg("request processed")
	})

	// 3. CORS Middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 4. Rate Limiting Middleware (100 req/min = ~1.66 req/sec, burst 5)
	// We'll use 1.66/s refill rate.
	limiter := NewRateLimiter(rate.Limit(100.0/60.0), 5)
	r.Use(func(c *gin.Context) {
		l := limiter.GetLimiter(c.ClientIP())
		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, NewErrorResponse("rate limit exceeded"))
			return
		}
		c.Next()
	})

	// Define Routes
	api := r.Group("/api/v1")
	{
		tasks := api.Group("/tasks")
		{
			tasks.POST("", cfg.TaskHandler.CreateTask)
			tasks.POST("/bulk", cfg.TaskHandler.CreateBulkTask)
			tasks.GET("/:id", cfg.TaskHandler.GetTask)
			tasks.DELETE("/:id", cfg.TaskHandler.CancelTask)
		}

		api.GET("/metrics", cfg.TaskHandler.GetMetrics)
	}

	// Separate health check endpoint usually goes to root or dedicated path
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}
