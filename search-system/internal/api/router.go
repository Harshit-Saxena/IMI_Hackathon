package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yourusername/search-system/internal/dataset"
	"github.com/yourusername/search-system/internal/metrics"
	"github.com/yourusername/search-system/internal/outbox"
	"github.com/yourusername/search-system/internal/search"
	"github.com/yourusername/search-system/internal/upsert"
)

// prometheusMiddleware records request count and latency for every route.
// Uses c.FullPath() so labels contain route patterns, not param values
// (e.g. "/datasets/:id/search" not "/datasets/abc123/search").
func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		dur := time.Since(start).Seconds()

		metrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(dur)
	}
}

// NewRouter creates and returns the configured Gin engine with all routes registered.
func NewRouter(
	db *sql.DB,
	ob *outbox.Writer,
	cfg upsert.Config,
	metaStore *dataset.MetaStore,
	searchRouter *search.SmartSearchRouter,
) *gin.Engine {
	h := New(db, ob, cfg, metaStore, searchRouter)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(prometheusMiddleware())

	// ── Infrastructure ─────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "phase": "10"})
	})

	// ── Prometheus metrics ─────────────────────────────────────────────────
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// ── Dataset management ─────────────────────────────────────────────────
	r.POST("/datasets", h.CreateDataset)

	// ── Record operations ──────────────────────────────────────────────────
	datasets := r.Group("/datasets/:id")
	{
		// Phase 2: idempotent partial upsert (caller controls sync_token)
		datasets.POST("/records/bulk", h.BulkUpsert)

		// Phase 3: full source-of-truth sync — upsert batch + soft-delete stale
		datasets.POST("/records/sync", h.FullSync)

		// Phase 3: explicit single-record soft delete
		datasets.DELETE("/records/:record_id", h.DeleteRecord)

		// Phase 5: smart fuzzy search — routes to Bleve (memory/file) or ES based on tier
		datasets.GET("/search", h.Search)
	}

	return r
}
