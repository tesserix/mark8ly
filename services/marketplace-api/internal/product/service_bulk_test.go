package product

import (
	"testing"
)

func TestValidBulkActions_AllRecognized(t *testing.T) {
	expected := []BulkAction{
		BulkActionArchive,
		BulkActionUnarchive,
		BulkActionPublish,
		BulkActionUnpublish,
		BulkActionDelete,
		BulkActionAssignCategory,
	}
	for _, a := range expected {
		if !ValidBulkActions[a] {
			t.Errorf("expected %q to be a valid bulk action", a)
		}
	}
}

func TestValidBulkActions_RejectsUnknown(t *testing.T) {
	if ValidBulkActions["nope"] {
		t.Error("expected unknown action to be invalid")
	}
}

func TestMaxBulkIDs_Is100(t *testing.T) {
	if MaxBulkIDs != 100 {
		t.Errorf("expected MaxBulkIDs=100, got %d", MaxBulkIDs)
	}
}
