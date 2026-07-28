package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// memRepo is an in-memory csvjob.Repository for handler unit tests.
type memRepo struct {
	jobs map[string]*csvjob.CsvImportJob
}

func newMemRepo() *memRepo { return &memRepo{jobs: make(map[string]*csvjob.CsvImportJob)} }

func (r *memRepo) Create(_ context.Context, job *csvjob.CsvImportJob) error {
	r.jobs[job.ID] = job
	return nil
}

func (r *memRepo) GetByID(_ context.Context, id string) (*csvjob.CsvImportJob, error) {
	j, ok := r.jobs[id]
	if !ok {
		return nil, apperrors.NotFound("csv_import_job")
	}
	return j, nil
}

func (r *memRepo) ListByStore(_ context.Context, storeID string, _, _ int) ([]csvjob.CsvImportJob, int64, error) {
	var out []csvjob.CsvImportJob
	for _, j := range r.jobs {
		if j.StoreID == storeID {
			out = append(out, *j)
		}
	}
	return out, int64(len(out)), nil
}

func (r *memRepo) UpdateStatus(_ context.Context, id, status string) error {
	j, ok := r.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	j.Status = status
	return nil
}

func (r *memRepo) UpdateProgress(_ context.Context, id string, lastRow, sc, ec int) error {
	j, ok := r.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	j.LastProcessedRow = lastRow
	j.SuccessCount = sc
	j.ErrorCount = ec
	return nil
}

func (r *memRepo) UpdateHeartbeat(_ context.Context, id string) error { return nil }

func (r *memRepo) FindOrphanedJobs(_ context.Context, _ time.Duration) ([]csvjob.CsvImportJob, error) {
	return nil, nil
}

func (r *memRepo) FindQueuedJobs(_ context.Context, _ int) ([]csvjob.CsvImportJob, error) {
	return nil, nil
}

func (r *memRepo) FindByContentHash(_ context.Context, storeID, hash string) (*csvjob.CsvImportJob, error) {
	for _, j := range r.jobs {
		if j.StoreID == storeID && j.ContentHash == hash &&
			(j.Status == csvjob.StatusQueued || j.Status == csvjob.StatusRunning || j.Status == csvjob.StatusPaused) {
			return j, nil
		}
	}
	return nil, nil
}

func (r *memRepo) SetStatusFields(_ context.Context, id string, fields map[string]any) error {
	j, ok := r.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	if s, ok := fields["status"]; ok {
		j.Status = s.(string)
	}
	return nil
}

// stubExportRepo satisfies product.ExportRepository for handler tests.
type stubExportRepo struct{}

func (stubExportRepo) ForEachExportRow(_ context.Context, _, _ string, _ func(product.ExportRow) error) error {
	return nil
}

func TestCSVImportsHandler_SubmitAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	svc := csvjob.NewService(repo, nil)
	handler := admin.NewCSVImportsHandler(svc, stubExportRepo{}, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Set("tenant_id", "test-tenant")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)
	r.GET("/csv-imports/:id", handler.Status)

	// Test 1: POST without file returns 400.
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/csv-imports", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Test 2: POST with file returns 201 + job.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte("title,handle\nShirt,shirt\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/csv-imports", &body)
	req2.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusCreated, w2.Code)

	var resp admin.CSVImportJobResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Equal(t, csvjob.StatusQueued, resp.Status)
	require.NotEmpty(t, resp.ID)
	require.Equal(t, "store-1", resp.StoreID)

	// Test 3: GET status returns the same job.
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "/csv-imports/"+resp.ID, nil)
	r.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusOK, w3.Code)

	var statusResp admin.CSVImportJobResponse
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &statusResp))
	require.Equal(t, resp.ID, statusResp.ID)
	require.Equal(t, csvjob.StatusQueued, statusResp.Status)
}

func TestCSVImportsHandler_SubmitDeduplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	svc := csvjob.NewService(repo, nil)
	handler := admin.NewCSVImportsHandler(svc, stubExportRepo{}, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Set("tenant_id", "test-tenant")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)

	csvData := []byte("title,handle\nShirt,shirt\n")

	submit := func() *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "test.csv")
		part.Write(csvData)
		writer.Close()

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/csv-imports", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		r.ServeHTTP(w, req)
		return w
	}

	// First submit: 201.
	w1 := submit()
	require.Equal(t, http.StatusCreated, w1.Code)

	// Second submit with same content: 200 (deduplicated).
	w2 := submit()
	require.Equal(t, http.StatusOK, w2.Code)

	var r1, r2 admin.CSVImportJobResponse
	json.Unmarshal(w1.Body.Bytes(), &r1)
	json.Unmarshal(w2.Body.Bytes(), &r2)
	require.Equal(t, r1.ID, r2.ID)
}
