package db

import (
	"os"
	"testing"

	"github.com/sourcegraph/sourcegraph/pkg/db/dbconn"
	"github.com/sourcegraph/sourcegraph/pkg/db/dbtesting"
)

func TestMigrations(t *testing.T) {
	if os.Getenv("SKIP_MIGRATION_TEST") != "" {
		t.Skip()
	}

	// Setup a global test database
	dbtesting.SetupGlobalTestDB(t)

	m := dbconn.NewMigrate(dbconn.Global)
	// Run all down migrations then up migrations again to ensure there are no SQL errors.
	if err := m.Down(); err != nil {
		t.Errorf("error running down migrations: %s", err)
	}
	if err := dbconn.DoMigrate(m); err != nil {
		t.Errorf("error running up migrations: %s", err)
	}
}

func TestPassword(t *testing.T) {
	// By default we use fast mocks for our password in tests. This ensures
	// our actual implementation is correct.
	oldHash := dbtesting.MockHashPassword
	oldValid := dbtesting.MockValidPassword
	dbtesting.MockHashPassword = nil
	dbtesting.MockValidPassword = nil
	defer func() {
		dbtesting.MockHashPassword = oldHash
		dbtesting.MockValidPassword = oldValid
	}()

	h, err := hashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if !validPassword(h.String, "correct-password") {
		t.Fatal("validPassword should of returned true")
	}
	if validPassword(h.String, "wrong-password") {
		t.Fatal("validPassword should of returned false")
	}
}
