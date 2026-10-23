package continuecmd_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/continuecmd"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/continue")
}
