package xargs_test

import (
	"testing"

	_ "github.com/FourCoreLabs/go-bash/builtins/echo"
	_ "github.com/FourCoreLabs/go-bash/builtins/xargs"
	"github.com/FourCoreLabs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/xargs")
}
