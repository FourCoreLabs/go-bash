package help_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/help"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/help")
}
