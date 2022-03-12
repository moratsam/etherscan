package cdb

import (
	"database/sql"
	"os"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/moratsam/etherscan/scorestore/test"
)

var _ = gc.Suite(new(CDBScoreStoreTestSuite))

func Test(t *testing.T) { gc.TestingT(t) }

type CDBScoreStoreTestSuite struct {
	test.SuiteBase
	db *sql.DB
}

func (s *CDBScoreStoreTestSuite) SetUpSuite(c *gc.C) {
	dsn := os.Getenv("CDB_DSN")
	if dsn == "" {
		c.Skip("Missing CDB_DSN envvar; skipping cockroachdb-backed scorestore test suite")
	}

	g, err := NewCDBScoreStore(dsn)
	c.Assert(err, gc.IsNil)
	s.SetScoreStore(g)
	s.db = g.db
}

func (s *CDBScoreStoreTestSuite) SetUpTest(c *gc.C) {
	s.flushDB(c)
}

func (s *CDBScoreStoreTestSuite) TearDownSuite(c *gc.C) {
	if s.db != nil {
		s.flushDB(c)
		c.Assert(s.db.Close(), gc.IsNil)
	}
}

func (s *CDBScoreStoreTestSuite) flushDB(c *gc.C) {
	_, err := s.db.Exec("delete from score")
	c.Assert(err, gc.IsNil)
	_, err = s.db.Exec("delete from scorer")
	c.Assert(err, gc.IsNil)
}
