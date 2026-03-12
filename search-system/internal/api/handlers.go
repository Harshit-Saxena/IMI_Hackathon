package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/search-system/internal/outbox"
	"github.com/yourusername/search-system/internal/upsert"
)

// createDatasetSQL inserts a dataset and initialises its state and count rows atomically.
const createDatasetSQL = `
WITH ins AS (
    INSERT INTO datasets (name, source) VALUES ($1, $2) RETURNING id
),
_state AS (
    INSERT INTO dataset_states (dataset_id)
    SELECT id FROM ins
),
_counts AS (
    INSERT INTO dataset_counts (dataset_id, record_count)
    SELECT id, 0 FROM ins
)
SELECT id FROM ins`

// Handler holds all HTTP route handlers.
type Handler struct {
	db     *sql.DB
	engine *upsert.UpsertEngine
}

// New constructs a Handler.
func New(db *sql.DB, ob *outbox.Writer, cfg upsert.Config) *Handler {
	return &Handler{
		db:     db,
		engine: upsert.New(db, ob, cfg),
	}
}

// CreateDataset handles POST /datasets
//
// Request:  {"name": "products_catalog", "source": "erp_system"}
// Response: {"id": "<uuid>", "name": "...", "source": "..."}
func (h *Handler) CreateDataset(c *gin.Context) {
	var req struct {
		Name   string `json:"name"   binding:"required"`
		Source string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var id string
	err := h.db.QueryRowContext(c.Request.Context(), createDatasetSQL, req.Name, req.Source).Scan(&id)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "dataset already exists or db error: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name, "source": req.Source})
}

// bulkUpsertRequest is the JSON body for POST /datasets/:id/records/bulk
type bulkUpsertRequest struct {
	SyncToken string          `json:"sync_token"`
	Records   []upsert.Record `json:"records" binding:"required,min=1"`
}

// fullSyncRequest is the JSON body for POST /datasets/:id/records/sync
type fullSyncRequest struct {
	Records []upsert.Record `json:"records" binding:"required,min=1"`
}

// BulkUpsert handles POST /datasets/:id/records/bulk
//
// Request:
//
//	{
//	  "sync_token": "batch-2024-01",   // optional — used for soft-delete purge in Phase 3
//	  "records": [
//	    {"external_id": "PROD-001", "source": "erp", "name": "Blue Widget", "value": {...}}
//	  ]
//	}
//
// Response: UpsertResult JSON with counts and duration.
func (h *Handler) BulkUpsert(c *gin.Context) {
	datasetID := c.Param("id")

	var req bulkUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Stamp the sync token onto every record so the purge query can identify this batch.
	if req.SyncToken != "" {
		for i := range req.Records {
			req.Records[i].SyncToken = req.SyncToken
		}
	}

	result := h.engine.BulkUpsert(c.Request.Context(), datasetID, req.Records)

	status := http.StatusOK
	if result.Failed > 0 && result.Failed == result.Total {
		status = http.StatusInternalServerError
	}
	c.JSON(status, result)
}

// FullSync handles POST /datasets/:id/records/sync
//
// Performs a full source-of-truth synchronisation:
//   1. Generates a unique sync token for this batch.
//   2. Upserts all provided records.
//   3. Soft-deletes any record in the dataset NOT present in this batch.
//
// Use this endpoint when you have the complete current state of a dataset.
// Records absent from the payload are treated as deleted in the source system.
//
// Request:  {"records": [...]}
// Response: SyncResult JSON with upsert counts, deleted count, sync_token, duration.
func (h *Handler) FullSync(c *gin.Context) {
	datasetID := c.Param("id")

	var req fullSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.engine.FullSync(c.Request.Context(), datasetID, req.Records)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// deleteRecordSQL soft-deletes a single record by its deterministic ID.
const deleteRecordSQL = `
UPDATE records
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id         = $1
  AND dataset_id = $2
  AND deleted_at IS NULL`

// DeleteRecord handles DELETE /datasets/:id/records/:record_id
//
// Soft-deletes a single record. The record remains in PostgreSQL but is
// invisible to all search queries. Use FullSync for bulk deletions.
func (h *Handler) DeleteRecord(c *gin.Context) {
	datasetID := c.Param("id")
	recordID := c.Param("record_id")

	res, err := h.db.ExecContext(c.Request.Context(), deleteRecordSQL, recordID, datasetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "record not found or already deleted"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": recordID})
}
