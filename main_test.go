package main

import (
	"crypto/subtle"
	"reflect"
	"testing"
)

func Test_generateSha256(t *testing.T) {
	type args struct {
		token   []byte
		payload []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "Github Example",
			args: args{
				token:   []byte("It's a Secret to Everybody"),
				payload: []byte("Hello, World!"),
			},
			want: []byte("757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateSha256(tt.args.token, tt.args.payload)
			if err != nil {
				t.Errorf("generateSha256() error = %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateSha256() = \n%v\nwant\n%v", got, tt.want)
			}

			compare := subtle.ConstantTimeCompare(got, tt.want)
			if compare != 1 {
				t.Errorf("subtle.ConstantTimeCompare() = got: %v, want: %v", compare, 1)
			}
		})
	}
}
