package ssz

import (
	"reflect"
	"testing"
)

type varsItem struct {
	HistoricalRoots [][]byte `ssz-size:"?,2" ssz-max:"2"`
}

func TestMarshalVar(t *testing.T) {
	item := varsItem{
		HistoricalRoots: [][]byte{
			{1, 2},
			{3, 4},
		},
	}
	s := determineSize(reflect.ValueOf(item))
	t.Logf("Size: %d", s)
	v, err := Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Marshaled is %v", v)
	var original varsItem
	if err := Unmarshal(v, &original); err != nil {
		t.Fatal(err)
	}
	if !DeepEqual(item, original) {
		t.Errorf("Not equal %v %v", item, original)
	}
}
