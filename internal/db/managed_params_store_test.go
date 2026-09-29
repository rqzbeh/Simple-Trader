package db_test

import (
	"context"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/db"
)

func TestManagedParamsReport_NilPool(t *testing.T) {
	store := &db.Store{}
	_, err := store.ManagedParamsReport(context.Background(), 14)
	if err == nil {
		t.Fatal("expected error with nil pool, got nil")
	}
}
