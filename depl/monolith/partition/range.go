package partition

import (
	"fmt"
	"math/big"
	"sort"

	"golang.org/x/xerrors"
)

// Range represents a contiguous wallet address region
// which is split into a number of partitions.
type Range struct {
	start			string
	rangeSplits	[]string
}

// NewFullRange creates a new range that uses the full wallet address value space
// and splits it into the provided number of partitions.
func NewFullRange(numPartitions int) (*Range, error) {
	return NewRange(
		"0000000000000000000000000000000000000000",
		"ffffffffffffffffffffffffffffffffffffffff",
		numPartitions,
	)
}

// NewRange creates a new range [minAddr, maxAddr)
// and splits it into the provided number of partitions.
func NewRange(minAddr, maxAddr string, numPartitions int) (*Range, error) {
	var (
		ok bool
		boundary = big.NewInt(0)
		minAddrInt = big.NewInt(0)
		maxAddrInt = big.NewInt(0)
		ranges = make([]string, numPartitions)
	)

	if minAddr >= maxAddr {
		return nil, xerrors.Errorf("range min address must be less than max address")
	} else if numPartitions <= 0 {
		return nil, xerrors.Errorf("number of partitions must exceed 0")
	} else if minAddrInt, ok = minAddrInt.SetString(minAddr, 16); !ok {
		return nil, xerrors.Errorf("failed to parse min address: %s", minAddr)
	}  else if maxAddrInt, ok = maxAddrInt.SetString(maxAddr, 16); !ok {
		return nil, xerrors.Errorf("failed to parse max address: %s", maxAddr)
	}

	// Calculate the size of each partition as ((maxAddr - minAddr + 1) / numPartitions).
	partSize := big.NewInt(0)
	partSize = partSize.Sub(maxAddrInt, minAddrInt)
	partSize = partSize.Div(partSize.Add(partSize, big.NewInt(1)), big.NewInt(int64(numPartitions)))

	for partition := 0; partition < numPartitions; partition++ {
		if partition == numPartitions-1 {
			ranges[partition] = maxAddr
		} else {
			boundary.Mul(partSize, big.NewInt(int64(partition+1)))
			ranges[partition] = fmt.Sprintf("%x", boundary) // convert to hex string.
		}
	}

	return &Range{start: minAddr, rangeSplits: ranges}, nil
}

// Extents returns the [start, end) extents of the entire range.
func (r *Range) Extents() (string, string) {
	return r.start, r.rangeSplits[len(r.rangeSplits)-1]
}

// PartitionExtents returns the [minAddr, maxAddr) range for the requested partition.
func (r *Range) PartitionExtents(partition int) (string, string, error) {
	if partition < 0 || partition >= len(r.rangeSplits) {
		return "", "", xerrors.Errorf("invalid partition index")
	}

	if partition == 0 {
		return r.start, r.rangeSplits[0], nil
	}
	return r.rangeSplits[partition-1], r.rangeSplits[partition], nil
}


// PartitionForAddr returns the partition index that the provided address belongs to.
func (r *Range) PartitionForAddr(address string) (int, error) {
	// As our partition ranges are already sorted we can run a binary search to
	// find the correct partition slot.
	partIndex := sort.Search(len(r.rangeSplits), func(n int) bool {
		return address < r.rangeSplits[n]
	})

	if address < r.start || partIndex >= len(r.rangeSplits) {
		return -1, xerrors.Errorf("unable to detect partition for address %q", address)
	}

	return partIndex, nil
}
