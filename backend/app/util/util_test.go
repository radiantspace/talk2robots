package util

import (
	"reflect"
	"strings"
	"testing"
)

func TestChunkString(t *testing.T) {
	type args struct {
		s         string
		chunkSize int
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "Empty",
			args: args{
				s:         "",
				chunkSize: 10,
			},
			want: []string{},
		},
		{
			name: "Single line",
			args: args{
				s:         "This is a single line",
				chunkSize: 10,
			},
			want: []string{
				"This is a",
				"single",
				"line",
			},
		},
		{
			name: "Multiple lines",
			args: args{
				s:         "This is a single line\nThis is a second line",
				chunkSize: 10,
			},
			want: []string{
				"This is a",
				"single",
				"line\n",
				"This is a",
				"second",
				"line",
			},
		},
		{
			name: "Long line",
			args: args{
				s:         "This is a long line that will be split by words",
				chunkSize: 10,
			},
			want: []string{
				"This is a",
				"long line",
				"that will",
				"be split",
				"by words",
			},
		},
		{
			name: "Long line with newlines",
			args: args{
				s:         "This is a long line that will be split by words\nThis is a second line that will be split by words",
				chunkSize: 10,
			},
			want: []string{
				"This is a",
				"long line",
				"that will",
				"be split",
				"by words\n",
				"This is a",
				"second",
				"line that",
				"will be",
				"split by",
				"words",
			},
		},
		{
			name: "Long line with newlines and no split",
			args: args{
				s:         "This is a long line that will not be split by words\nThis is a second line that will not be split by words",
				chunkSize: 256,
			},
			want: []string{
				"This is a long line that will not be split by words\nThis is a second line that will not be split by words",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChunkString(tt.args.s, tt.args.chunkSize); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\nRESULT:\n%s\nEXPECTED:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}
