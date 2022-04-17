package service

import (
	"encoding/binary"
	"math"

	"github.com/gogo/protobuf/types"
	"golang.org/x/xerrors"
)

type serializer struct {
}

// Serialize encodes the given value into an types.Any protobuf message.
func (serializer) Serialize(v interface{}) (*types.Any, error) {
	scratchBuf := make([]byte, binary.MaxVarintLen64)
	switch val := v.(type) {
	case int:
		nBytes := binary.PutVarint(scratchBuf, int64(val))
		return &types.Any{
			TypeUrl: "i",
			Value:   scratchBuf[:nBytes],
		}, nil
	case float64:
		nBytes := binary.PutUvarint(scratchBuf, math.Float64bits(val))
		return &types.Any{
			TypeUrl: "f",
			Value:   scratchBuf[:nBytes],
		}, nil
	default:
		return nil, xerrors.Errorf("serialize: unknown type %#+T", val)
	}
}

// Unserialize decodes the given types.Any protobuf value.
func (serializer) Unserialize(v *types.Any) (interface{}, error) {
	switch v.TypeUrl {
	case "i":
		val, _ := binary.Varint(v.Value)
		return int(val), nil
	case "f":
		val, _ := binary.Uvarint(v.Value)
		return math.Float64frombits(val), nil
	default:
		return nil, xerrors.Errorf("unserialize: unknown type %q", v.TypeUrl)
	}
}
