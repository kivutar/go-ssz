package ssz

import (
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
	if _, err := newMakeUnmarshaler(input, rval.Elem(), 0); err != nil {
		return fmt.Errorf("could not unmarshal input into type: %v, %v", rval.Elem().Type(), err)
	}
	return nil
}

func newMakeUnmarshaler(input []byte, val reflect.Value, startOffset uint64) (uint64, error) {
	typ := val.Type()
	kind := typ.Kind()
	switch {
	case kind == reflect.Bool:
		return unmarshalBool(input, val, startOffset)
	case kind == reflect.Uint8:
		return unmarshalUint8(input, val, startOffset)
	case kind == reflect.Uint16:
		return unmarshalUint16(input, val, startOffset)
	case kind == reflect.Uint32:
		return unmarshalUint32(input, val, startOffset)
	case kind == reflect.Int32:
		return unmarshalUint32(input, val, startOffset)
	case kind == reflect.Uint64:
		return unmarshalUint64(input, val, startOffset)
	//case kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint8:
	//	return makeByteSliceUnmarshaler()
	//case kind == reflect.Array && typ.Elem().Kind() == reflect.Uint8:
	//	return makeBasicArrayUnmarshaler(typ)
	//case kind == reflect.Slice && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
	//	return makeBasicSliceUnmarshaler(typ)
	//case kind == reflect.Slice && isBasicType(typ.Elem().Kind()):
	//	return makeBasicSliceUnmarshaler(typ)
	//case kind == reflect.Slice && !isVariableSizeType(typ.Elem()):
	//	return makeBasicSliceUnmarshaler(typ)
	//case kind == reflect.Array && !isVariableSizeType(typ.Elem()):
	//	return makeBasicArrayUnmarshaler(typ)
	//case kind == reflect.Slice:
	//	return makeCompositeSliceUnmarshaler(typ)
	//case kind == reflect.Array:
	//	return makeCompositeArrayUnmarshaler(typ)
	//case kind == reflect.Struct:
	//	return makeStructUnmarshaler(typ)
	//case kind == reflect.Ptr:
	//	return makePtrUnmarshaler(typ)
	default:
		return 0, fmt.Errorf("type %v is not deserializable", typ)
	}
}
