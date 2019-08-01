package ssz

import (
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
)

// NewUnmarshal SSZ encoded data and output it into the object pointed by pointer val.
// Given a struct with the following fields, and some encoded bytes of type []byte,
// one can then unmarshal the bytes into a pointer of the struct as follows:
//  type exampleStruct1 struct {
//      Field1 uint8
//      Field2 []byte
//  }
//
//  var targetStruct exampleStruct1
//  if err := Unmarshal(encodedBytes, &targetStruct); err != nil {
//      return fmt.Errorf("failed to unmarshal: %v", err)
//  }
func NewUnmarshal(input []byte, val interface{}) error {
	if val == nil {
		return errors.New("cannot unmarshal into untyped, nil value")
	}
	rval := reflect.ValueOf(val)
	rtyp := rval.Type()
	// val must be a pointer, otherwise we refuse to unmarshal
	if rtyp.Kind() != reflect.Ptr {
		return errors.New("can only unmarshal into a pointer target")
	}
	if rval.IsNil() {
		return errors.New("cannot output to pointer of nil value")
	}
	if _, err := newMakeUnmarshaler(input, rval.Elem(), rval.Elem().Type(), 0); err != nil {
		return fmt.Errorf("could not unmarshal input into type: %v, %v", rval.Elem().Type(), err)
	}
	return nil
}

func newMakeUnmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	kind := typ.Kind()
	switch {
	//case kind == reflect.Bool:
	//	return unmarshalBool(input, val, typ, startOffset)
	//case kind == reflect.Uint8:
	//	return unmarshalUint8(input, val, typ, startOffset)
	//case kind == reflect.Uint16:
	//	return unmarshalUint16(input, val, typ, startOffset)
	//case kind == reflect.Uint32:
	//	return unmarshalUint32(input, val, typ, startOffset)
	//case kind == reflect.Int32:
	//	return unmarshalUint32(input, val, typ, startOffset)
	//case kind == reflect.Uint64:
	//	return unmarshalUint64(input, val, typ, startOffset)
	case kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint8:
		return newByteSliceUnmarshaler(input, val, typ, startOffset)
	case kind == reflect.Array && typ.Elem().Kind() == reflect.Uint8:
		return newBasicArrayUnmarshaler(input, val, typ, startOffset)
	case kind == reflect.Slice && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
		return newBasicSliceUmmarshaler(input, val, typ, startOffset)
	case kind == reflect.Slice && isBasicType(typ.Elem().Kind()):
		return newBasicSliceUmmarshaler(input, val, typ, startOffset)
	case kind == reflect.Slice && !isVariableSizeType(typ.Elem()):
		return newBasicSliceUmmarshaler(input, val, typ, startOffset)
	case kind == reflect.Array && !isVariableSizeType(typ.Elem()):
		return newBasicArrayUnmarshaler(input, val, typ, startOffset)
	case kind == reflect.Slice:
		return newCompositeSliceUnmarshaler(input, val, typ, startOffset)
	case kind == reflect.Array:
		return newCompositeArrayUnmarshaler(input, val, typ, startOffset)
	//case kind == reflect.Struct:
	//	return makeStructUnmarshaler(typ)
	//case kind == reflect.Ptr:
	//	return makePtrUnmarshaler(typ)
	default:
		return 0, fmt.Errorf("type %v is not deserializable", typ)
	}
}

func newByteSliceUnmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	offset := startOffset + uint64(len(input))
	val.SetBytes(input[startOffset:offset])
	return offset, nil
}

func newBasicSliceUmmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	if len(input) == 0 {
		newVal := reflect.MakeSlice(val.Type(), 0, 0)
		val.Set(newVal)
		return 0, nil
	}
	// If there are struct tags that specify a different type, we handle accordingly.
	if val.Type() != typ {
		sizes := []uint64{1}
		innerElement := typ.Elem()
		for {
			if innerElement.Kind() == reflect.Slice {
				sizes = append(sizes, 0)
				innerElement = innerElement.Elem()
			} else if innerElement.Kind() == reflect.Array {
				sizes = append(sizes, uint64(innerElement.Len()))
				innerElement = innerElement.Elem()
			} else {
				break
			}
		}
		// If the item is a slice, we grow it accordingly based on the size tags.
		result := growSliceFromSizeTags(val, sizes)
		reflect.Copy(result, val)
		val.Set(result)
	} else {
		growConcreteSliceType(val, val.Type(), 1)
	}

	var err error
	index := startOffset
	index, err = newMakeUnmarshaler(input, val.Index(0), val.Index(0).Type(), index)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal element of slice: %v", err)
	}

	elementSize := index - startOffset
	endOffset := uint64(len(input)) / elementSize
	if val.Type() != typ {
		sizes := []uint64{endOffset}
		innerElement := typ.Elem()
		for {
			if innerElement.Kind() == reflect.Slice {
				sizes = append(sizes, 0)
				innerElement = innerElement.Elem()
			} else if innerElement.Kind() == reflect.Array {
				sizes = append(sizes, uint64(innerElement.Len()))
				innerElement = innerElement.Elem()
			} else {
				break
			}
		}
		// If the item is a slice, we grow it accordingly based on the size tags.
		result := growSliceFromSizeTags(val, sizes)
		reflect.Copy(result, val)
		val.Set(result)
	}
	i := uint64(1)
	for i < endOffset {
		if val.Type() == typ {
			growConcreteSliceType(val, val.Type(), int(i)+1)
		}
		index, err = newMakeUnmarshaler(input, val.Index(int(i)), typ.Elem(), index)
		if err != nil {
			return 0, fmt.Errorf("failed to unmarshal element of slice: %v", err)
		}
		i++
	}
	return index, nil
}

func newCompositeSliceUnmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	if len(input) == 0 {
		newVal := reflect.MakeSlice(val.Type(), 0, 0)
		val.Set(newVal)
		return 0, nil
	}
	growConcreteSliceType(val, typ, 1)
	endOffset := uint64(len(input))

	currentIndex := startOffset
	nextIndex := currentIndex
	offsetVal := input[startOffset : startOffset+BytesPerLengthOffset]
	firstOffset := startOffset + uint64(binary.LittleEndian.Uint32(offsetVal))
	currentOffset := firstOffset
	nextOffset := currentOffset
	i := 0
	for currentIndex < firstOffset {
		nextIndex = currentIndex + BytesPerLengthOffset
		if nextIndex == firstOffset {
			nextOffset = endOffset
		} else {
			nextOffsetVal := input[nextIndex : nextIndex+BytesPerLengthOffset]
			nextOffset = startOffset + uint64(binary.LittleEndian.Uint32(nextOffsetVal))
		}
		// We grow the slice's size to accommodate a new element being unmarshaled.
		growConcreteSliceType(val, typ, i+1)
		if _, err := newMakeUnmarshaler(input[currentOffset:nextOffset], val.Index(i), typ.Elem(), 0); err != nil {
			return 0, fmt.Errorf("failed to unmarshal element of slice: %v", err)
		}
		i++
		currentIndex = nextIndex
		currentOffset = nextOffset
	}
	return currentIndex, nil
}

func newBasicArrayUnmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	i := 0
	index := startOffset
	size := val.Len()
	var err error
	for i < size {
		if val.Index(i).Kind() == reflect.Ptr {
			instantiateConcreteTypeForElement(val.Index(i), typ.Elem().Elem())
		}
		index, err = newMakeUnmarshaler(input, val.Index(i), typ.Elem(), index)
		if err != nil {
			return 0, fmt.Errorf("failed to unmarshal element of array: %v", err)
		}
		i++
	}
	return index, nil
}

func newCompositeArrayUnmarshaler(input []byte, val reflect.Value, typ reflect.Type, startOffset uint64) (uint64, error) {
	currentIndex := startOffset
	nextIndex := currentIndex
	offsetVal := input[startOffset : startOffset+BytesPerLengthOffset]
	firstOffset := startOffset + uint64(binary.LittleEndian.Uint32(offsetVal))
	currentOffset := firstOffset
	nextOffset := currentOffset
	endOffset := uint64(len(input))
	i := 0
	for currentIndex < firstOffset {
		nextIndex = currentIndex + BytesPerLengthOffset
		if nextIndex == firstOffset {
			nextOffset = endOffset
		} else {
			nextOffsetVal := input[nextIndex : nextIndex+BytesPerLengthOffset]
			nextOffset = startOffset + uint64(binary.LittleEndian.Uint32(nextOffsetVal))
		}
		if val.Index(i).Kind() == reflect.Ptr {
			instantiateConcreteTypeForElement(val.Index(i), typ.Elem().Elem())
		}
		if _, err := newMakeUnmarshaler(input[currentOffset:nextOffset], val.Index(i), typ.Elem(), 0); err != nil {
			return 0, fmt.Errorf("failed to unmarshal element of slice: %v", err)
		}
		i++
		currentIndex = nextIndex
		currentOffset = nextOffset
	}
	return currentIndex, nil
}
