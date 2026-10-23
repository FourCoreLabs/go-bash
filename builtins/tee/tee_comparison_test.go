package tee_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/cat"
	_ "github.com/FourCoreLabs/go-bash/builtins/tee"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/tee")
}
