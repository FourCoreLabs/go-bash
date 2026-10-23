package tr_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/tr"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/tr")
}
