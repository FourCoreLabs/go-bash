package zcat_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/gzip"
	_ "github.com/FourCoreLabs/go-bash/builtins/zcat"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/zcat")
}
