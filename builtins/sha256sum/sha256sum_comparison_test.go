package sha256sum_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/sha256sum"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/sha256sum")
}
