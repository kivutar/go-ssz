package ssz

import (
	"reflect"
	"testing"
)

type sampleItem struct {
	Slot  uint64
	IsNew bool
	Root  []byte
}

func TestNewMarshal(t *testing.T) {
	type args struct {
		val interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "some struct",
			args: args{
				val: sampleItem{
					Slot:  4,
					IsNew: true,
					Root:  []byte{1, 2, 3, 4},
				},
			},
			want:    []byte{1},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMarshal(tt.args.val)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMarshal() got = %v, want %v", got, tt.want)
			}
		})
	}
}
