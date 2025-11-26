package krab

import (
	"github.com/qbart/ferrodb/krabhcl"
)

// TestSuite represents test runner configuration.
type TestSuite struct {
	Tests   []*TestExample
}

func (t *TestSuite) Addr() krabhcl.Addr {
	return krabhcl.NullAddr
}

func (t *TestSuite) Validate() error {
	return nil
}
