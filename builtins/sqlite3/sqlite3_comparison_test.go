package sqlite3_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/sqlite3"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/sqlite3-stub")
}
