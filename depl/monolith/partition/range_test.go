package partition

import (
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(new(RangeTestSuite))

type RangeTestSuite struct {
}

func (s *RangeTestSuite) TestNewRangeErrors(c *gc.C) {
	_, err := NewRange( "f", "a", 1)
	c.Assert(err, gc.ErrorMatches, "range min address must be less than max address")

	_, err = NewRange( "a", "f", 0)
	c.Assert(err, gc.ErrorMatches, "number of partitions must exceed 0")
}

func (s *RangeTestSuite) TestEvenSplit(c *gc.C) {
	r, err := NewFullRange(4)
	c.Assert(err, gc.IsNil)

	expExtents := [][2]string{
		{"0000000000000000000000000000000000000000", "4000000000000000000000000000000000000000"},
		{"4000000000000000000000000000000000000000", "8000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000", "c000000000000000000000000000000000000000"},
		{"c000000000000000000000000000000000000000", "ffffffffffffffffffffffffffffffffffffffff"},
	}

	for i, exp := range expExtents {
		c.Logf("extent: %d", i)
		gotFrom, gotTo, err := r.PartitionExtents(i)
		c.Assert(err, gc.IsNil)
		c.Assert(gotFrom, gc.Equals, exp[0])
		c.Assert(gotTo, gc.Equals, exp[1])
	}
}

func (s *RangeTestSuite) TestOddSplit(c *gc.C) {
	r, err := NewFullRange(3)
	c.Assert(err, gc.IsNil)

	expExtents := [][2]string{
		{"0000000000000000000000000000000000000000", "5555555555555555555555555555555555555555"},
		{"5555555555555555555555555555555555555555", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "ffffffffffffffffffffffffffffffffffffffff"},
	}

	for i, exp := range expExtents {
		c.Logf("extent: %d", i)
		gotFrom, gotTo, err := r.PartitionExtents(i)
		c.Assert(err, gc.IsNil)
		c.Assert(gotFrom, gc.Equals, exp[0])
		c.Assert(gotTo, gc.Equals, exp[1])
	}
}

func (s *RangeTestSuite) TestPartitionExtentsError(c *gc.C) {
	r, err := NewRange("a", "f", 1)
	c.Assert(err, gc.IsNil)

	_, _, err = r.PartitionExtents(1)
	c.Assert(err, gc.ErrorMatches, "invalid partition index")
}


func (s *RangeTestSuite) TestPartitionLookup(c *gc.C) {
	r, err := NewFullRange(4)
	c.Assert(err, gc.IsNil)

	from, to := r.Extents()
	c.Assert(from, gc.Equals, "0000000000000000000000000000000000000000")
	c.Assert(to, gc.Equals, "ffffffffffffffffffffffffffffffffffffffff")

	specs := []struct {
		addr  string
		exp int
	}{
		{"0f00000000000000000000000000000000000000", 0},
		{"4000000000000000000000000000000000000000", 1},
		{"8000000010000000000000000000000000000000", 2},
		{"c000000030000000000000000000000000000000", 3},
	}

	for i, spec := range specs {
		c.Logf("spec: %d -> lookup partition for %v", i, spec.addr)
		got, err := r.PartitionForAddr(spec.addr)
		c.Assert(err, gc.IsNil)
		c.Assert(got, gc.Equals, spec.exp)
	}
}

func (s *RangeTestSuite) TestPartitionLookupError(c *gc.C) {
	r, err := NewRange(
		"c000000000000000000000000000000000000000",
		"f000000000000000000000000000000000000000",
		1,
	)
	c.Assert(err, gc.IsNil)

	_, err = r.PartitionForAddr("0000000000000000000000000000000000000000")
	c.Assert(err, gc.ErrorMatches, "unable to detect partition.*")

	_, err = r.PartitionForAddr("ffffffffffffffffffffffffffffffffffffffff")
	c.Assert(err, gc.ErrorMatches, "unable to detect partition.*")
}
