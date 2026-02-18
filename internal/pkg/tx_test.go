package pkg

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// --- mock infrastructure ---

// mockTxPool implements gorm.ConnPool + gorm.TxCommitter, acting as the
// "transaction connection" returned by BeginTx.
type mockTxPool struct {
	committed  bool
	rolledBack bool
}

func (m *mockTxPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, nil
}
func (m *mockTxPool) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (m *mockTxPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockTxPool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return nil
}
func (m *mockTxPool) Commit() error   { m.committed = true; return nil }
func (m *mockTxPool) Rollback() error { m.rolledBack = true; return nil }

// mockBeginner implements gorm.ConnPool + gorm.ConnPoolBeginner.
type mockBeginner struct {
	txPool   *mockTxPool
	beginErr error
}

func (m *mockBeginner) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, nil
}
func (m *mockBeginner) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (m *mockBeginner) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockBeginner) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return nil
}
func (m *mockBeginner) BeginTx(_ context.Context, _ *sql.TxOptions) (gorm.ConnPool, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	return m.txPool, nil
}

// newMockDB creates a *gorm.DB backed by the mock ConnPoolBeginner.
func newMockDB(beginner *mockBeginner) *gorm.DB {
	db := &gorm.DB{Config: &gorm.Config{}}
	db.Statement = &gorm.Statement{
		DB:       db,
		ConnPool: beginner,
	}
	return db
}

// --- tests ---

func TestWithTx_CommitOnSuccess(t *testing.T) {
	txPool := &mockTxPool{}
	db := newMockDB(&mockBeginner{txPool: txPool})

	err := WithTx(db, func(tx *gorm.DB) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !txPool.committed {
		t.Fatal("expected Commit to be called")
	}
	if txPool.rolledBack {
		t.Fatal("Rollback should not be called on success")
	}
}

func TestWithTx_RollbackOnFnError(t *testing.T) {
	txPool := &mockTxPool{}
	db := newMockDB(&mockBeginner{txPool: txPool})

	fnErr := errors.New("fn failed")
	err := WithTx(db, func(tx *gorm.DB) error {
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Fatalf("expected fn error, got %v", err)
	}
	if !txPool.rolledBack {
		t.Fatal("expected Rollback to be called")
	}
	if txPool.committed {
		t.Fatal("Commit should not be called on fn error")
	}
}

func TestWithTx_RollbackAndRepanic(t *testing.T) {
	txPool := &mockTxPool{}
	db := newMockDB(&mockBeginner{txPool: txPool})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to be re-raised")
		}
		if r != "boom" {
			t.Fatalf("expected panic value 'boom', got %v", r)
		}
		if !txPool.rolledBack {
			t.Fatal("expected Rollback on panic")
		}
		if txPool.committed {
			t.Fatal("Commit should not be called on panic")
		}
	}()

	WithTx(db, func(tx *gorm.DB) error {
		panic("boom")
	})
}

func TestWithTx_BeginError(t *testing.T) {
	beginErr := errors.New("begin failed")
	db := newMockDB(&mockBeginner{beginErr: beginErr})

	err := WithTx(db, func(tx *gorm.DB) error {
		t.Fatal("fn should not be called when Begin fails")
		return nil
	})
	if err == nil {
		t.Fatal("expected error from Begin, got nil")
	}
}

// --- SQLite integration tests ---

// testItem is a simple model for integration tests.
type testItem struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:100"`
}

// newTxTestDB creates a SQLite in-memory *gorm.DB and auto-migrates testItem.
func newTxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&testItem{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestWithTx_SQLite_CommitOnSuccess(t *testing.T) {
	db := newTxTestDB(t)

	err := WithTx(db, func(tx *gorm.DB) error {
		return tx.Create(&testItem{Name: "alice"}).Error
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var count int64
	db.Model(&testItem{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after commit, got %d", count)
	}

	var item testItem
	db.First(&item)
	if item.Name != "alice" {
		t.Fatalf("expected name 'alice', got %q", item.Name)
	}
}

func TestWithTx_SQLite_RollbackOnError(t *testing.T) {
	db := newTxTestDB(t)

	fnErr := errors.New("something went wrong")
	err := WithTx(db, func(tx *gorm.DB) error {
		if err := tx.Create(&testItem{Name: "bob"}).Error; err != nil {
			t.Fatalf("insert should succeed: %v", err)
		}
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Fatalf("expected fn error, got %v", err)
	}

	var count int64
	db.Model(&testItem{}).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}

func TestWithTx_SQLite_RollbackOnPanic(t *testing.T) {
	db := newTxTestDB(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to be re-raised")
		}
		if r != "kaboom" {
			t.Fatalf("expected panic value 'kaboom', got %v", r)
		}

		// Verify row was rolled back.
		var count int64
		db.Model(&testItem{}).Count(&count)
		if count != 0 {
			t.Fatalf("expected 0 rows after panic rollback, got %d", count)
		}
	}()

	WithTx(db, func(tx *gorm.DB) error {
		if err := tx.Create(&testItem{Name: "charlie"}).Error; err != nil {
			t.Fatalf("insert should succeed: %v", err)
		}
		panic("kaboom")
	})
}
