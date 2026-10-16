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

// Weight (grams) and parcel size (cm) have to reach the variant: carriers
// quote on it.
func TestWorker_SavesWeightAndSizeOnTheVariant(t *testing.T) {
	repo := newFakeRepo()
	creator := &fakeProductCreator{}
	job := &csvjob.CsvImportJob{ID: "job-w", StoreID: "store-1", UserID: "user-1",
		GCSPath: "w.csv", ContentHash: "hash-w", Status: csvjob.StatusQueued}
	require.NoError(t, repo.Create(context.Background(), job))

	csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: "title,handle,base_price,sku,stock,weight,length_cm,width_cm,height_cm\nMug,mug,45.00,M1,3,500,15,15,12.5\n"},
		ErrFactory:   newFakeErrorWriterFactory(),
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "AUD",
	}).Run(context.Background(), *job)

	require.Len(t, creator.calls, 1)
	got := creator.calls[0].Variants[0].WeightGrams
	require.NotNil(t, got)
	require.Equal(t, 500, *got)
	v := creator.calls[0].Variants[0]
	require.Equal(t, "15", v.LengthCM.String())
	require.Equal(t, "15", v.WidthCM.String())
	require.Equal(t, "12.5", v.HeightCM.String())
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

// TestOrphanWindowIsFifteenMinutes pins the shared WINDOW behind "this job
// is orphaned". The health endpoint (#289) and the startup recovery scan
// both read this constant, so retuning it moves both together. Their
// boundary predicates differ by one instant on purpose (`<=` vs `<`) — see
// the constant's doc comment.
func TestOrphanWindowIsFifteenMinutes(t *testing.T) {
	require.Equal(t, 15*time.Minute, csvjob.OrphanWindow)
}

// runImport runs one import of csvContent and returns the job and fakes.
func runImport(t *testing.T, csvContent string, creator *fakeProductCreator) (*csvjob.CsvImportJob, *fakeErrorWriterFactory) {
	t.Helper()
	repo := newFakeRepo()
	job := &csvjob.CsvImportJob{ID: "job-o", StoreID: "store-1", UserID: "user-1",
		GCSPath: "o.csv", ContentHash: "hash-o", Status: csvjob.StatusQueued}
	require.NoError(t, repo.Create(context.Background(), job))
	errFactory := newFakeErrorWriterFactory()
	csvjob.NewWorker(csvjob.WorkerConfig{
		Repo: repo, ProductSvc: creator, CSVReader: &fakeCSVReader{content: csvContent},
		ErrFactory: errFactory, StoreID: "store-1", TenantID: "tenant-1", CurrencyCode: "AUD",
	}).Run(context.Background(), *job)
	final, err := repo.GetByID(context.Background(), job.ID)
	require.NoError(t, err)
	return final, errFactory
}

// Rows sharing a handle are one product, a variant per row, laid out the
// way a Shopify export is: title and option names only on the first row.
func TestWorker_RowsSharingAHandleAreOneProductWithVariants(t *testing.T) {
	creator := &fakeProductCreator{}
	final, errs := runImport(t, "title,handle,base_price,sku,stock,weight,option1_name,option1_value,option2_name,option2_value\n"+
		"Mug,mug,45.00,MUG-S-SAND,3,400,Size,Small,Colour,Sand\n"+
		",mug,45.00,MUG-S-BLUE,2,400,,Small,,Cornflower\n"+
		",mug,52.00,MUG-L-SAND,1,550,,Large,,Sand\n"+
		",mug,52.00,MUG-L-BLUE,0,550,,Large,,Cornflower\n"+
		"Towel,towel,24.00,TOWEL,9,120,Title,Default Title,,\n", creator)

	require.Equal(t, csvjob.StatusCompleted, final.Status)
	require.Empty(t, errs.writer.rows)
	require.Equal(t, 5, final.SuccessCount)
	require.Len(t, creator.calls, 2)

	mug := creator.calls[0]
	require.Equal(t, "Mug", mug.Title)
	require.Equal(t, []product.OptionSpec{
		{Name: "Size", Values: []product.OptionValueSpec{{Value: "Small"}, {Value: "Large"}}},
		{Name: "Colour", Values: []product.OptionValueSpec{{Value: "Sand"}, {Value: "Cornflower"}}},
	}, mug.Options)
	require.Len(t, mug.Variants, 4)
	require.Equal(t, "MUG-L-BLUE", mug.Variants[3].SKU)
	require.Equal(t, "52", mug.Variants[3].Price.String())
	require.Equal(t, 550, *mug.Variants[3].WeightGrams)
	require.Equal(t, []product.OptionValueRef{{OptionName: "Size", Value: "Large"}, {OptionName: "Colour", Value: "Cornflower"}},
		mug.Variants[3].OptionValues)

	// The real service accepts the shape: it rejects a gappy option grid.
	specs := make([]product.VariantSpec, len(mug.Variants))
	for i, v := range mug.Variants {
		specs[i] = product.VariantSpec{SKU: v.SKU, OptionValues: v.OptionValues}
	}
	require.NoError(t, product.ValidateMatrix(mug.Options, specs))

	// Shopify's placeholder for "no options" is not an option.
	require.Empty(t, creator.calls[1].Options)
	require.Empty(t, creator.calls[1].Variants[0].OptionValues)
}

// One bad variant row fails its whole product, and every row of it lands
// in the error file, so the merchant re-imports the product, not a part.
func TestWorker_ABadVariantRowFailsItsWholeProduct(t *testing.T) {
	creator := &fakeProductCreator{}
	final, errs := runImport(t, "title,handle,base_price,sku,option1_name,option1_value\n"+
		"Mug,mug,45.00,MUG-SAND,Colour,Sand\n"+
		",mug,45.00,MUG-BLUE,,\n"+
		"Bowl,bowl,30.00,BOWL,,\n", creator)

	require.Equal(t, 1, final.SuccessCount)
	require.Equal(t, 2, final.ErrorCount)
	require.Len(t, creator.calls, 1)
	require.Equal(t, "Bowl", creator.calls[0].Title)
	require.Len(t, errs.writer.rows, 2)
	msg := errs.writer.rows[0][len(errs.writer.rows[0])-1]
	require.Contains(t, msg, "row 2: option1_value is required for every variant")
}

// Rows join a product only while they are consecutive; the same handle
// further down is a second product with that handle, which is refused.
func TestWorker_AHandleReusedLaterIsADuplicate(t *testing.T) {
	creator := &fakeProductCreator{}
	final, errs := runImport(t, "title,handle,base_price,sku\n"+
		"Mug,mug,45.00,M1\n"+
		"Bowl,bowl,30.00,B1\n"+
		"Mug again,mug,45.00,M2\n", creator)

	require.Equal(t, 2, final.SuccessCount)
	require.Equal(t, 1, final.ErrorCount)
	require.Contains(t, errs.writer.rows[0][len(errs.writer.rows[0])-1], "duplicate handle")
}
