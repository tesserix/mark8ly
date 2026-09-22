package admin

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CSVUploader persists the uploaded CSV body so the worker can read it
// back at the job's gcs_path.
type CSVUploader interface {
	Upload(ctx context.Context, path string, r io.Reader) error
}

// CSVImportsHandler bundles dependencies for the CSV import/export endpoints.
type CSVImportsHandler struct {
	svc        *csvjob.Service
	exportRepo product.ExportRepository
	uploader   CSVUploader
	logger     *slog.Logger
}

// NewCSVImportsHandler constructs a CSVImportsHandler. uploader may be nil
// only where object storage is not configured; Submit then refuses rather
// than queueing a job whose file does not exist (#897).
func NewCSVImportsHandler(svc *csvjob.Service, exportRepo product.ExportRepository, uploader CSVUploader, logger *slog.Logger) *CSVImportsHandler {
	return &CSVImportsHandler{svc: svc, exportRepo: exportRepo, uploader: uploader, logger: logger}
}

// Submit handles POST /admin/stores/:storeId/csv-imports.
// Accepts a multipart file upload, computes sha256, and submits the job.
func (h *CSVImportsHandler) Submit(c *gin.Context) {
	storeID := c.Param("storeId")
	userID := c.GetString("user_id")

	if h.uploader == nil {
		RespondErr(c, fmt.Errorf("csv_imports: object storage is not configured"), h.logger)
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("file", "file is required"), h.logger)
		return
	}
	defer file.Close()

	// Compute sha256 while reading — we need the hash before creating the job.
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		RespondErr(c, fmt.Errorf("csv_imports: hash file: %w", err), h.logger)
		return
	}
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	gcsPath := fmt.Sprintf("csv-imports/%s/%s.csv", storeID, contentHash)

	// Rewind: hashing consumed the reader, and the same bytes are what we
	// store. Upload before creating the row — a queued job pointing at a
	// missing object can only ever fail.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		RespondErr(c, fmt.Errorf("csv_imports: rewind upload: %w", err), h.logger)
		return
	}
	if err := h.uploader.Upload(c.Request.Context(), gcsPath, file); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	result, err := h.svc.SubmitWithHash(c.Request.Context(), storeID, userID, gcsPath, contentHash)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	status := http.StatusCreated
	if result.Deduplicated {
		status = http.StatusOK
	}
	c.JSON(status, ToCSVImportJobResponse(&result.Job))
}

// List handles GET /admin/stores/:storeId/csv-imports.
func (h *CSVImportsHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")

	var q ListCSVImportsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}
	q.Defaults()

	jobs, total, err := h.svc.ListByStore(c.Request.Context(), storeID, q.Page, q.PageSize)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	items := make([]CSVImportJobResponse, 0, len(jobs))
	for i := range jobs {
		items = append(items, ToCSVImportJobResponse(&jobs[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": total,
		"page":  q.Page,
	})
}

// Status handles GET /admin/stores/:storeId/csv-imports/:id.
func (h *CSVImportsHandler) Status(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.svc.GetStatus(c.Request.Context(), jobID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToCSVImportJobResponse(job))
}

// Cancel handles POST /admin/stores/:storeId/csv-imports/:id/cancel.
func (h *CSVImportsHandler) Cancel(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.svc.Cancel(c.Request.Context(), jobID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// DownloadErrors handles GET /admin/stores/:storeId/csv-imports/:id/errors.
// Streams the error CSV from GCS as text/csv. Currently returns 501 when
// GCS streaming is not wired (deferred to CI with real GCS).
func (h *CSVImportsHandler) DownloadErrors(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.svc.GetStatus(c.Request.Context(), jobID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if job.ErrorCSVGCSPath == nil || *job.ErrorCSVGCSPath == "" {
		RespondErr(c, apperrors.NotFound("error_csv"), h.logger)
		return
	}

	// GCS download would go here. For now, return 501.
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "error CSV download requires GCS integration (available in CI)",
	})
}

// Export handles GET /admin/stores/:storeId/products/export.csv.
// Streams a CSV of all products in the store.
func (h *CSVImportsHandler) Export(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"products-%s.csv\"", storeID[:8]))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// Write header.
	if err := writer.Write([]string{"id", "handle", "title", "description", "status", "sku", "base_price", "stock"}); err != nil {
		h.logger.Error("csv export: write header", "err", err)
		return
	}

	err := h.exportRepo.ForEachExportRow(c.Request.Context(), storeID, tenantID, func(row product.ExportRow) error {
		desc := ""
		if row.Description != nil {
			desc = *row.Description
		}
		// Escape every user-supplied text column to defuse spreadsheet
		// formula injection (=cmd|, @SUM, +HYPERLINK, ...). See
		// audit_logs.go:escapeCSVCell for rationale. Numeric columns are
		// safe because fmt.Sprintf cannot introduce a leading formula char.
		if writeErr := writer.Write([]string{
			escapeCSVCell(row.ID),
			escapeCSVCell(row.Handle),
			escapeCSVCell(row.Title),
			escapeCSVCell(desc),
			escapeCSVCell(row.Status),
			escapeCSVCell(row.SKU),
			escapeCSVCell(row.Price),
			fmt.Sprintf("%d", row.Stock),
		}); writeErr != nil {
			return writeErr
		}
		// Flush periodically to avoid buffering the entire export in memory.
		writer.Flush()
		return writer.Error()
	})
	if err != nil {
		h.logger.Error("csv export: stream rows", "err", err)
	}
}
