package ssz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"

	"github.com/prysmaticlabs/go-bitfield"
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
	kind := typ.Kind()
	switch {
	case isBasicType(kind) || isBasicTypeArray(typ, kind):
		return makeBasicTypeHasher(typ)
	case kind == reflect.Slice && isBasicType(typ.Elem().Kind()):
		return makeBasicSliceHasher(typ)
	case kind == reflect.Slice && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
		return makeBasicSliceHasher(typ)
	case kind == reflect.Array && isBasicTypeArray(typ.Elem(), typ.Elem().Kind()):
		return makeBasicArrayHasher(typ)
	case kind == reflect.Slice && !isBasicType(typ.Elem().Kind()):
		return makeCompositeSliceHasher(typ)
	case kind == reflect.Array:
		return makeCompositeArrayHasher(typ)
	case kind == reflect.Struct:
		return makeStructHasher(typ)
	case kind == reflect.Ptr:
		return makePtrHasher(typ)
	default:
		return nil, fmt.Errorf("type %v is not hashable", typ)
	}
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

func newBasicArrayHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	var leaves [][]byte
	for i := 0; i < val.Len(); i++ {
		r, err := newMakeHasher(val.Index(i), 0)
		if err != nil {
			return [32]byte{}, err
		}
		leaves = append(leaves, r[:])
	}
	chunks, err := pack(leaves)
	if err != nil {
		return [32]byte{}, err
	}
	if val.Len() == 0 {
		chunks = [][]byte{}
	}
	return bitwiseMerkleize(chunks, 1, false /* has limit */)
}

func newCompositeArrayHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	roots := [][]byte{}
	elemSize := uint64(0)
	if isBasicType(val.Type().Elem().Kind()) {
		elemSize = determineFixedSize(val, val.Type().Elem())
	} else {
		elemSize = 32
	}
	limit := (uint64(val.Len())*elemSize + 31) / 32
	for i := 0; i < val.Len(); i++ {
		r, err := newMakeHasher(val.Index(i), 0)
		if err != nil {
			return [32]byte{}, err
		}
		roots = append(roots, r[:])
	}
	chunks, err := pack(roots)
	if err != nil {
		return [32]byte{}, err
	}
	if val.Len() == 0 {
		chunks = [][]byte{}
	}
	return bitwiseMerkleize(chunks, limit, true /* has limit */)
}

func newBasicSliceHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	elemSize := uint64(0)
	if isBasicType(val.Type().Elem().Kind()) {
		elemSize = determineFixedSize(val, val.Type().Elem())
	} else {
		elemSize = 32
	}
	limit := (maxCapacity*elemSize + 31) / 32
	if limit == 0 {
		limit = 1
	}

	var leaves [][]byte
	var err error
	for i := 0; i < val.Len(); i++ {
		if isBasicType(val.Index(i).Kind()) {
			innerBufSize := determineSize(val.Index(i))
			innerBuf := make([]byte, innerBufSize)
			if _, err = newMakeMarshaler(val.Index(i), innerBuf, 0); err != nil {
				return [32]byte{}, err
			}
			leaves = append(leaves, innerBuf)
		} else {
			r, err := newMakeHasher(val.Index(i), 0)
			if err != nil {
				return [32]byte{}, err
			}
			leaves = append(leaves, r[:])
		}
	}
	chunks, err := pack(leaves)
	if err != nil {
		return [32]byte{}, err
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint64(val.Len()))
	output := make([]byte, 32)
	copy(output, buf.Bytes())
	merkleRoot, err := bitwiseMerkleize(chunks, limit, true /* has limit */)
	if err != nil {
		return [32]byte{}, err
	}
	return mixInLength(merkleRoot, output), nil
}

func newCompositeSliceHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	roots := [][]byte{}
	output := make([]byte, 32)
	if val.Len() == 0 && maxCapacity == 0 {
		merkleRoot, err := bitwiseMerkleize([][]byte{}, 0, true /* has limit */)
		if err != nil {
			return [32]byte{}, err
		}
		itemMerkleize := mixInLength(merkleRoot, output)
		return itemMerkleize, nil
	}
	var err error
	for i := 0; i < val.Len(); i++ {
		var r [32]byte
		if useCache {
			r, err = hashCache.lookup(val.Index(i), newMakeHasher, newMakeMarshaler, 0)
		} else {
			r, err = newMakeHasher(val.Index(i), 0)
		}
		if err != nil {
			return [32]byte{}, err
		}
		roots = append(roots, r[:])
	}
	chunks, err := pack(roots)
	if err != nil {
		return [32]byte{}, err
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint64(val.Len()))
	copy(output, buf.Bytes())
	objLen := maxCapacity
	if maxCapacity == 0 {
		objLen = uint64(val.Len())
	}
	merkleRoot, err := bitwiseMerkleize(chunks, objLen, true /* has limit */)
	if err != nil {
		return [32]byte{}, err
	}
	return mixInLength(merkleRoot, output), nil
}

func newMakeStructHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	roots := [][]byte{}
	for i := 0; i < val.Type().NumField(); i++ {
		var r [32]byte
		var err error
		if _, ok := val.Field(i).Interface().(bitfield.Bitlist); ok {
			r, err = bitlistHasher(val.Field(i), 0 /* TODO: ADD CAPACITY */)
			roots = append(roots, r[:])
			continue
		}
		if useCache {
			r, err = hashCache.lookup(
				val.Field(i),
				newMakeHasher,
				newMakeMarshaler,
				0, /* TODO: ADD CAPACITY */
			)
		} else {
			r, err = newMakeHasher(val.Field(i), 0 /* TODO: ADD CAPACITY */)
		}
		if err != nil {
			return [32]byte{}, fmt.Errorf("failed to hash field %s of struct: %v", val.Field(i).Type().Name(), err)
		}
		roots = append(roots, r[:])
	}
	return bitwiseMerkleize(roots, uint64(val.Type().NumField()), true /* has limit */)
}

func newMakePtrHasher(val reflect.Value, maxCapacity uint64) ([32]byte, error) {
	if val.IsNil() {
		return [32]byte{}, nil
	}
	return newMakeHasher(val.Elem(), maxCapacity)
}
