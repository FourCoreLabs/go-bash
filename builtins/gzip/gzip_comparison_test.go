package gzip_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/gunzip"
	_ "github.com/FourCoreLabs/go-bash/builtins/gzip"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/gzip")
}
