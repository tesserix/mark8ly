package delivery

import "testing"

func TestTableNames(t *testing.T) {
	if (Option{}).TableName() != "store_delivery_options" || (Courier{}).TableName() != "store_couriers" {
		t.Fatal("unexpected table name")
	}
}
