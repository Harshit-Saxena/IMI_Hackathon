package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/search-system/internal/outbox"
	"github.com/yourusername/search-system/internal/upsert"
)

// NewRouter creates and returns the configured Gin engine with all routes registered.
func NewRouter(db *sql.DB, ob *outbox.Writer, cfg upsert.Config) *gin.Engine {
	h := New(db, ob, cfg)

	r := gin.New()
	r.Use(gin.Recovery())

	// ── Infrastructure ─────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "phase": "3"})
	})

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
	}

	return r
}
