package partition

import (
	"fmt"
	"math/big"

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
func NewFullRange(numPartitions int) (Range, error) {
	return NewRange(
		"0000000000000000000000000000000000000000",
		"ffffffffffffffffffffffffffffffffffffffff",
		numPartitions,
	)
}

// NewRange creates a new range [minAddr, maxAddr)
// and splits it into the provided number of partitions.
func NewRange(minAddr, maxAddr string, numPartitions int) (Range, error) {
	var (
		ok bool
		boundary = big.NewInt(0)
		minAddrInt = big.NewInt(0)
		maxAddrInt = big.NewInt(0)
		ranges = make([]string, numPartitions)
	)

	if minAddr >= maxAddr {
		return Range{}, xerrors.Errorf("range min address must be less than max address")
	} else if numPartitions <= 0 {
		return Range{}, xerrors.Errorf("number of partitions must exceed 0")
	} else if minAddrInt, ok = minAddrInt.SetString(minAddr, 16); !ok {
		return Range{}, xerrors.Errorf("failed to parse min address: %s", minAddr)
	}  else if maxAddrInt, ok = maxAddrInt.SetString(maxAddr, 16); !ok {
		return Range{}, xerrors.Errorf("failed to parse max address: %s", maxAddr)
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

	return Range{start: minAddr, rangeSplits: ranges}, nil
}

// PartitionExtents returns the [minAddr, maxAddr) range for the requested partition.
func (r Range) PartitionExtents(partition int) (string, string, error) {
	if partition < 0 || partition >= len(r.rangeSplits) {
		return "", "", xerrors.Errorf("invalid partition index")
	}

	if partition == 0 {
		return r.start, r.rangeSplits[0], nil
	}
	return r.rangeSplits[partition-1], r.rangeSplits[partition], nil
}
