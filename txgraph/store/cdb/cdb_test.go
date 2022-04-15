package cdb

import (
	"database/sql"
	"os"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/txgraph/graph/test"
)

var _ = gc.Suite(new(CDBGraphTestSuite))

func Test(t *testing.T) { gc.TestingT(t) }

type CDBGraphTestSuite struct {
	test.SuiteBase
	db *sql.DB
}

func (s *CDBGraphTestSuite) SetUpSuite(c *gc.C) {
	dsn := os.Getenv("CDB_DSN")
	if dsn == "" {
		c.Skip("Missing CDB_DSN envvar; skipping cockroachdb-backed graph test suite")
	}

	g, err := NewCDBGraph(dsn)
	c.Assert(err, gc.IsNil)
	s.SetGraph(g)
	s.db = g.db
}

func (s *CDBGraphTestSuite) SetUpTest(c *gc.C) {
	s.flushDB(c)
}

func (s *CDBGraphTestSuite) TearDownSuite(c *gc.C) {
	if s.db != nil {
		s.flushDB(c)
		c.Assert(s.db.Close(), gc.IsNil)
	}
}

func (s *CDBGraphTestSuite) flushDB(c *gc.C) {
	_, err := s.db.Exec("delete from block")
	c.Assert(err, gc.IsNil)
	_, err = s.db.Exec("delete from tx")
	c.Assert(err, gc.IsNil)
	_, err = s.db.Exec("delete from wallet")
	c.Assert(err, gc.IsNil)
}
