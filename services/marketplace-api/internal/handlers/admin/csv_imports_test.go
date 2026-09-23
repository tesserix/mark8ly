package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
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
	handler := admin.NewCSVImportsHandler(svc, stubExportRepo{}, newMemUploader(), nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Set("tenant_id", "test-tenant")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)
	// Status is mounted with :storeId, as production does — the handler
	// refuses a job that is not this store's.
	r.GET("/stores/:storeId/csv-imports/:id", handler.Status)

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
	req3 := httptest.NewRequest("GET", "/stores/store-1/csv-imports/"+resp.ID, nil)
	r.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusOK, w3.Code)

	var statusResp admin.CSVImportJobResponse
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &statusResp))
	require.Equal(t, resp.ID, statusResp.ID)
	require.Equal(t, csvjob.StatusQueued, statusResp.Status)

	// Test 4: the same job id under another store is not this caller's to
	// read. The error CSV quotes the importing merchant's own product rows,
	// so a leak here is a leak of their catalogue.
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("GET", "/stores/store-2/csv-imports/"+resp.ID, nil)
	r.ServeHTTP(w4, req4)
	require.Equal(t, http.StatusNotFound, w4.Code)
}

func TestCSVImportsHandler_SubmitDeduplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	svc := csvjob.NewService(repo, nil)
	handler := admin.NewCSVImportsHandler(svc, stubExportRepo{}, newMemUploader(), nil)

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

func (r *memRepo) ClaimJob(_ context.Context, id string) (bool, error) {
	j, ok := r.jobs[id]
	if !ok || j.Status != csvjob.StatusQueued {
		return false, nil
	}
	j.Status = csvjob.StatusRunning
	return true, nil
}

// memUploader records what Submit stored, so a test can assert the bytes
// actually landed at the gcs_path written on the job — the exact link that
// was missing when import looked like it worked (#897).
type memUploader struct {
	objects map[string][]byte
	err     error
}

func newMemUploader() *memUploader { return &memUploader{objects: map[string][]byte{}} }

func (u *memUploader) Upload(_ context.Context, path string, r io.Reader) error {
	if u.err != nil {
		return u.err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	u.objects[path] = b
	return nil
}

// The regression this whole issue was: the handler computed a gcs_path,
// discarded the uploaded bytes, and returned 201. The job was then
// unrunnable, but nothing said so.
func TestCSVImportsHandler_Submit_StoresFileAtJobPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	uploader := newMemUploader()
	handler := admin.NewCSVImportsHandler(csvjob.NewService(repo, nil), stubExportRepo{}, uploader, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)

	csvBody := "name,sku,price\nWidget,W-1,9.99\n"
	w := postCSV(t, r, csvBody)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		ID      string `json:"id"`
		GCSPath string `json:"gcs_path"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	job, err := repo.GetByID(context.Background(), resp.ID)
	require.NoError(t, err)

	stored, ok := uploader.objects[job.GCSPath]
	require.True(t, ok, "no object was stored at the job's gcs_path %q", job.GCSPath)
	require.Equal(t, csvBody, string(stored),
		"the stored bytes must be the uploaded CSV, not a truncated or re-read copy")
}

func TestCSVImportsHandler_Submit_FailsWhenUploadFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	uploader := newMemUploader()
	uploader.err = errors.New("bucket unavailable")
	handler := admin.NewCSVImportsHandler(csvjob.NewService(repo, nil), stubExportRepo{}, uploader, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)

	w := postCSV(t, r, "name,sku,price\nWidget,W-1,9.99\n")
	require.NotEqual(t, http.StatusCreated, w.Code,
		"a failed upload must not report the import as accepted")
	require.Empty(t, repo.jobs, "no job row should exist for a CSV that was never stored")
}

func TestCSVImportsHandler_Submit_RefusesWithoutStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	handler := admin.NewCSVImportsHandler(csvjob.NewService(repo, nil), stubExportRepo{}, nil, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)

	w := postCSV(t, r, "name,sku,price\nWidget,W-1,9.99\n")
	require.NotEqual(t, http.StatusCreated, w.Code)
	require.Empty(t, repo.jobs)
}

func postCSV(t *testing.T, r *gin.Engine, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "products.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/csv-imports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)
	return w
}

// postCSVWithMapping submits a CSV plus the mapper's payload.
func postCSVWithMapping(t *testing.T, r *gin.Engine, content, mapping string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "products.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	if mapping != "" {
		require.NoError(t, mw.WriteField("column_mapping", mapping))
	}
	require.NoError(t, mw.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/csv-imports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)
	return w
}

func mappingHandler(t *testing.T) (*gin.Engine, *memRepo, *memUploader) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newMemRepo()
	uploader := newMemUploader()
	handler := admin.NewCSVImportsHandler(csvjob.NewService(repo, nil), stubExportRepo{}, uploader, nil)

	r := gin.New()
	r.POST("/csv-imports", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.Submit)
	return r, repo, uploader
}

// The mapper was purely decorative: the page collected the merchant's
// choices and never sent them, so a CSV whose headers were not already
// canonical imported nothing.
func TestCSVImportsHandler_Submit_AppliesColumnMapping(t *testing.T) {
	r, repo, uploader := mappingHandler(t)

	w := postCSVWithMapping(t, r,
		"Product Name,URL Key,Variant Price\nShirt,shirt-1,10.00\n",
		`{"Product Name":"title","URL Key":"handle","Variant Price":"base_price"}`)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	job, err := repo.GetByID(context.Background(), resp.ID)
	require.NoError(t, err)

	require.Equal(t,
		"title,handle,base_price\nShirt,shirt-1,10.00\n",
		string(uploader.objects[job.GCSPath]),
		"the stored CSV must carry the canonical headers the parser reads")
}

func TestCSVImportsHandler_Submit_RejectsCollidingMapping(t *testing.T) {
	r, repo, uploader := mappingHandler(t)

	w := postCSVWithMapping(t, r,
		"Price,Sale Price\n10.00,8.00\n",
		`{"Price":"base_price","Sale Price":"base_price"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Empty(t, repo.jobs)
	require.Empty(t, uploader.objects, "nothing should be stored for a rejected mapping")
}

func TestCSVImportsHandler_Submit_RejectsMalformedMapping(t *testing.T) {
	r, repo, _ := mappingHandler(t)

	w := postCSVWithMapping(t, r, "title\nShirt\n", "not json")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Empty(t, repo.jobs)
}

// A merchant whose export already matches sends no mapping, and the file
// must reach the bucket untouched.
func TestCSVImportsHandler_Submit_WithoutMappingStoresFileVerbatim(t *testing.T) {
	r, repo, uploader := mappingHandler(t)

	csvBody := "title,handle,base_price\nShirt,shirt-1,10.00\n"
	w := postCSVWithMapping(t, r, csvBody, "")
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	job, err := repo.GetByID(context.Background(), resp.ID)
	require.NoError(t, err)
	require.Equal(t, csvBody, string(uploader.objects[job.GCSPath]))
}

// The admin client reads data/meta. While this returned items/total/page
// the import history silently rendered "No import history yet" with a
// 200 on the wire and jobs in the database — and the frontend's own test
// mocked data, so nothing caught the mismatch.
func TestCSVImportsHandler_List_UsesTheHouseEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMemRepo()
	require.NoError(t, repo.Create(context.Background(), &csvjob.CsvImportJob{
		ID: "job-1", StoreID: "store-1", Status: csvjob.StatusCompleted,
		GCSPath: "csv-imports/store-1/a.csv",
	}))
	handler := admin.NewCSVImportsHandler(csvjob.NewService(repo, nil), stubExportRepo{}, newMemUploader(), nil)

	r := gin.New()
	r.GET("/csv-imports", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "storeId", Value: "store-1"})
		c.Next()
	}, handler.List)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/csv-imports", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	data, ok := body["data"].([]any)
	require.True(t, ok, `list must return "data", got keys %v`, keysOf(body))
	require.Len(t, data, 1)

	meta, ok := body["meta"].(map[string]any)
	require.True(t, ok, `list must return "meta", got keys %v`, keysOf(body))
	for _, k := range []string{"page", "page_size", "total", "total_pages"} {
		require.Contains(t, meta, k)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
