package ssz

import (
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
)

// Marshal a value and output the result into a byte slice.
// Given a struct with the following fields, one can marshal it as follows:
//  type exampleStruct struct {
//      Field1 uint8
//      Field2 []byte
//  }
//
//  ex := exampleStruct{
//      Field1: 10,
//      Field2: []byte{1, 2, 3, 4},
//  }
//  encoded, err := Marshal(ex)
//  if err != nil {
//      return fmt.Errorf("failed to marshal: %v", err)
//  }
//
// One can also specify the specific size of a struct's field by using
// ssz-specific field tags as follows:
//
//  type exampleStruct struct {
//      Field1 uint8
//      Field2 []byte `ssz:"size=32"`
//  }
//
// This will treat `Field2` as as [32]byte array when marshaling. For unbounded
// fields or multidimensional slices, ssz size tags can also be used as follows:
//
//  type exampleStruct struct {
//      Field1 uint8
//      Field2 [][]byte `ssz:"size=?,32"`
//  }
//
// This will treat `Field2` as type [][32]byte when marshaling a
// struct of that type.
func NewMarshal(val interface{}) ([]byte, error) {
	if val == nil {
		return nil, errors.New("untyped-value nil cannot be marshaled")
	}
	rval := reflect.ValueOf(val)

	// We pre-allocate a buffer-size depending on the value's calculated total byte size.
	buf := make([]byte, determineSize(rval))
	if _, err := newMakeMarshaler(rval, buf, 0 /* start offset */); err != nil {
		return nil, fmt.Errorf("failed to marshal for type: %v", rval.Type())
	}
	return buf, nil
}

func newMakeMarshaler(val reflect.Value, buf []byte, startOffset uint64) (uint64, error) {
	kind := val.Type().Kind()
	switch {
	case kind == reflect.Bool:
		return marshalBool(val, buf, startOffset)
	case kind == reflect.Uint8:
		return marshalUint8(val, buf, startOffset)
	case kind == reflect.Uint16:
		return marshalUint16(val, buf, startOffset)
	case kind == reflect.Uint32:
		return marshalUint32(val, buf, startOffset)
	case kind == reflect.Uint64:
		return marshalUint64(val, buf, startOffset)
	case kind == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8:
		return marshalByteSlice(val, buf, startOffset)
	case kind == reflect.Array && val.Type().Elem().Kind() == reflect.Uint8:
		return marshalByteArray(val, buf, startOffset)
	case kind == reflect.Slice && isBasicTypeArray(val.Type().Elem(), val.Type().Elem().Kind()):
		return newmakeBasicSliceMarshaler(val, buf, startOffset)
	case kind == reflect.Slice && isBasicType(val.Type().Elem().Kind()):
		return newmakeBasicSliceMarshaler(val, buf, startOffset)
	case kind == reflect.Slice && !isVariableSizeType(val.Type().Elem()):
		return newmakeBasicSliceMarshaler(val, buf, startOffset)
	case kind == reflect.Slice || kind == reflect.Array:
		return newmakeCompositeSliceMarshaler(val, buf, startOffset)
	case kind == reflect.Struct:
		return newmakeStructMarshaler(val, buf, startOffset)
	case kind == reflect.Ptr:
		return newmakePtrMarshaler(val, buf, startOffset)
	default:
		return 0, fmt.Errorf("type %v is not serializable", val.Type())
	}
}

func newmakeBasicSliceMarshaler(val reflect.Value, buf []byte, startOffset uint64) (uint64, error) {
	index := startOffset
	var err error
	for i := 0; i < val.Len(); i++ {
		index, err = newMakeMarshaler(val.Index(i), buf, index)
		if err != nil {
			return 0, err
		}
	}
	return index, nil
}

func newmakeCompositeSliceMarshaler(val reflect.Value, buf []byte, startOffset uint64) (uint64, error) {
	index := startOffset
	var err error
	if !isVariableSizeType(val.Type()) {
		for i := 0; i < val.Len(); i++ {
			// If each element is not variable size, we simply encode sequentially and write
			// into the buffer at the last index we wrote at.
			index, err = newMakeMarshaler(val.Index(i), buf, index)
			if err != nil {
				return 0, err
			}
		}
	} else {
		fixedIndex := index
		currentOffsetIndex := startOffset + uint64(val.Len())*BytesPerLengthOffset
		nextOffsetIndex := currentOffsetIndex
		// If the elements are variable size, we need to include offset indices
		// in the serialized output list.
		for i := 0; i < val.Len(); i++ {
			nextOffsetIndex, err = newMakeMarshaler(val.Index(i), buf, currentOffsetIndex)
			if err != nil {
				return 0, err
			}
			// Write the offset.
			offsetBuf := make([]byte, BytesPerLengthOffset)
			binary.LittleEndian.PutUint32(offsetBuf, uint32(currentOffsetIndex-startOffset))
			copy(buf[fixedIndex:fixedIndex+BytesPerLengthOffset], offsetBuf)

			// We increase the offset indices accordingly.
			currentOffsetIndex = nextOffsetIndex
			fixedIndex += BytesPerLengthOffset
		}
		index = currentOffsetIndex
	}
	return index, nil
}

func newmakeStructMarshaler(val reflect.Value, buf []byte, startOffset uint64) (uint64, error) {
	fields, err := structFields(val.Type())
	if err != nil {
		return 0, err
	}
	fixedIndex := startOffset
	fixedLength := uint64(0)
	// For every field, we add up the total length of the items depending if they
	// are variable or fixed-size fields.
	for _, f := range fields {
		if isVariableSizeType(val.Field(f.index).Type()) {
			fixedLength += BytesPerLengthOffset
		} else {
			fixedLength += determineFixedSize(val.Field(f.index), val.Field(f.index).Type())
		}
	}
	currentOffsetIndex := startOffset + fixedLength
	nextOffsetIndex := currentOffsetIndex
	for _, f := range fields {
		if !isVariableSizeType(val.Field(f.index).Type()) {
			fixedIndex, err = newMakeMarshaler(val.Field(f.index), buf, fixedIndex)
			if err != nil {
				return 0, err
			}
		} else {
			nextOffsetIndex, err = newMakeMarshaler(val.Field(f.index), buf, currentOffsetIndex)
			if err != nil {
				return 0, err
			}
			// Write the offset.
			offsetBuf := make([]byte, BytesPerLengthOffset)
			binary.LittleEndian.PutUint32(offsetBuf, uint32(currentOffsetIndex-startOffset))
			copy(buf[fixedIndex:fixedIndex+BytesPerLengthOffset], offsetBuf)

			// We increase the offset indices accordingly.
			currentOffsetIndex = nextOffsetIndex
			fixedIndex += BytesPerLengthOffset
		}
	}
	return currentOffsetIndex, nil
}

func newmakePtrMarshaler(val reflect.Value, buf []byte, startOffset uint64) (uint64, error) {
	// Nil encodes to []byte{}.
	if val.IsNil() {
		return 0, nil
	}
	return newMakeMarshaler(val.Elem(), buf, startOffset)
}
