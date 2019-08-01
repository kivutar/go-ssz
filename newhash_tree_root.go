package ssz

import (
	"errors"
	"fmt"
	"reflect"
)

// NewHashTreeRoot determines the root hash using SSZ's merkleization.
// Given a struct with the following fields, one can tree hash it as follows:
//  type exampleStruct struct {
//      Field1 uint8
//      Field2 []byte
//  }
//
//  ex := exampleStruct{
//      Field1: 10,
//      Field2: []byte{1, 2, 3, 4},
//  }
//  root, err := HashTreeRoot(ex)
//  if err != nil {
//      return fmt.Errorf("failed to compute root: %v", err)
//  }
func NewHashTreeRoot(val interface{}) ([32]byte, error) {
	if val == nil {
		return [32]byte{}, errors.New("untyped nil is not supported")
	}
	rval := reflect.ValueOf(val)
	output, err := newMakeHasher(rval, 0)
	if err != nil {
		return [32]byte{}, fmt.Errorf("could not tree hash type: %v: %v", rval.Type(), err)
	}
	return output, nil
}

func newMakeHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	//kind := typ.Kind()
	//switch {
	//case isBasicType(kind) || isBasicTypeArray(typ, kind):
	//	return makeBasicTypeHasher(typ)
	//case kind == reflect.Slice && isBasicType(typ.Elem().Kind()):
	//	return makeBasicSliceHasher(typ)
	//case kind == reflect.Slice && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
	//	return makeBasicSliceHasher(typ)
	//case kind == reflect.Array && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
	//	return makeBasicArrayHasher(typ)
	//case kind == reflect.Slice && !isBasicType(typ.Elem().Kind()):
	//	return makeCompositeSliceHasher(typ)
	//case kind == reflect.Array:
	//	return makeCompositeArrayHasher(typ)
	//case kind == reflect.Struct:
	//	return makeStructHasher(typ)
	//case kind == reflect.Ptr:
	//	return makePtrHasher(typ)
	//default:
	//	return nil, fmt.Errorf("type %v is not hashable", typ)
	//}
	return [32]byte{}, nil
}

func newMakeBasicTypeHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	buf := make([]byte, determineSize(val))
	if _, err := newMakeMarshaler(val, buf, 0); err != nil {
		return [32]byte{}, err
	}
	chunks, err := pack([][]byte{buf})
	if err != nil {
		return [32]byte{}, err
	}
	return bitwiseMerkleize(chunks, 1, false /* has limit */)
}
