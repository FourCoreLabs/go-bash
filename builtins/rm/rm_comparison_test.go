package rm_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/rm"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/rm")
}
