package csvjob_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// fakeCSVReader returns an in-memory reader for the given CSV content.
type fakeCSVReader struct {
	content string
}

func (f *fakeCSVReader) Open(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.content)), nil
}

// fakeProductCreator tracks calls and optionally fails.
type fakeProductCreator struct {
	calls      []product.CreateRequest
	failHandle string // if set, Create returns error for this handle
}

func (f *fakeProductCreator) Create(_ context.Context, req product.CreateRequest) (*product.Aggregate, error) {
	f.calls = append(f.calls, req)
	if f.failHandle != "" && req.Handle == f.failHandle {
		return nil, &fakeCreateError{handle: req.Handle}
	}
	return &product.Aggregate{Product: product.Product{ID: "fake-id"}}, nil
}

type fakeCreateError struct{ handle string }

func (e *fakeCreateError) Error() string { return "create failed for " + e.handle }

// fakeErrorWriterFactory captures error rows in memory.
type fakeErrorWriterFactory struct {
	writer *fakeErrorWriter
}

func newFakeErrorWriterFactory() *fakeErrorWriterFactory {
	return &fakeErrorWriterFactory{writer: &fakeErrorWriter{}}
}

func (f *fakeErrorWriterFactory) New(_ context.Context, _ string) (csvjob.ErrorCSVWriter, error) {
	return f.writer, nil
}

type fakeErrorWriter struct {
	headers []string
	rows    [][]string
}

func (f *fakeErrorWriter) WriteHeader(h []string) error { f.headers = h; return nil }
func (f *fakeErrorWriter) WriteRow(row []string, errMsg string) error {
	f.rows = append(f.rows, append(row, errMsg))
	return nil
}
func (f *fakeErrorWriter) Close() error { return nil }

func TestWorker_ProcessesRowsWithOneError(t *testing.T) {
	csvContent := "title,handle,base_price,sku,stock\n" +
		"Shirt,shirt-1,10.00,S1,5\n" +
		"Pants,pants-1,20.00,P1,3\n" +
		"Bad,bad-1,10.00,B1,5\n" + // will fail at product create
		"Hat,hat-1,5.00,H1,2\n" +
		"Socks,socks-1,3.00,SK1,10\n"

	repo := newFakeRepo()
	creator := &fakeProductCreator{failHandle: "bad-1"}
	errFactory := newFakeErrorWriterFactory()

	// Seed a queued job in the repo.
	job := &csvjob.CsvImportJob{
		ID:          "job-1",
		StoreID:     "store-1",
		UserID:      "user-1",
		GCSPath:     "test.csv",
		ContentHash: "hash-1",
		Status:      csvjob.StatusQueued,
	}
	require.NoError(t, repo.Create(context.Background(), job))

	w := csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: csvContent},
		ErrFactory:   errFactory,
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "USD",
	})

	w.Run(context.Background(), *job)

	// Verify final state.
	final, err := repo.GetByID(context.Background(), "job-1")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusCompleted, final.Status)
	require.Equal(t, 4, final.SuccessCount)
	require.Equal(t, 1, final.ErrorCount)

	// Verify error CSV has 1 data row.
	require.Len(t, errFactory.writer.rows, 1)
	require.Contains(t, errFactory.writer.rows[0][len(errFactory.writer.rows[0])-1], "bad-1")

	// Verify product creator was called 5 times.
	require.Len(t, creator.calls, 5)
}

func TestWorker_CancellationMidBatch(t *testing.T) {
	// Build a CSV with 25 rows so we get at least one checkpoint at row 10.
	var buf bytes.Buffer
	buf.WriteString("title,handle,base_price,sku,stock\n")
	for i := 1; i <= 25; i++ {
		buf.WriteString("Product,handle-" + itoa(i) + ",10.00,SKU-" + itoa(i) + ",5\n")
	}

	repo := newFakeRepo()
	job := &csvjob.CsvImportJob{
		ID:          "job-cancel",
		StoreID:     "store-1",
		UserID:      "user-1",
		GCSPath:     "cancel.csv",
		ContentHash: "hash-cancel",
		Status:      csvjob.StatusQueued,
	}
	require.NoError(t, repo.Create(context.Background(), job))

	creator := &fakeProductCreator{}
	errFactory := newFakeErrorWriterFactory()

	w := csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: buf.String()},
		ErrFactory:   errFactory,
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "USD",
	})

	// Cancel the job after the first checkpoint (row 10).
	// Simulate: after the worker checkpoints at row 10, it checks cancel status.
	// We set the job to cancelled right before run, then the check at row 10 finds it.
	// Actually, let's use TestCancelAfterRow.
	w.TestCancelAfterRow = 15

	w.Run(context.Background(), *job)

	// Job should NOT be in a terminal state (simulated crash).
	final, err := repo.GetByID(context.Background(), "job-cancel")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusRunning, final.Status) // still running (crashed)
	require.Equal(t, 15, final.LastProcessedRow)
	// TestCancelAfterRow fires before processing row 15, so only 14 succeeded.
	require.Equal(t, 14, final.SuccessCount)

	// Verify only 14 products were created (row 15 was not processed).
	require.Len(t, creator.calls, 14)
}

func itoa(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// TestOrphanWindowIsFifteenMinutes pins the shared definition of "this job
// is orphaned". The health endpoint (#289) and the startup recovery scan
// both read this constant; if they ever disagree, the console reports a
// job healthy that the recovery scan is about to reset.
func TestOrphanWindowIsFifteenMinutes(t *testing.T) {
	require.Equal(t, 15*time.Minute, csvjob.OrphanWindow)
}
