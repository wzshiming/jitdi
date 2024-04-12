package handler

import (
	"reflect"
	"testing"
)

func Test_parseSegments(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    []segment
		wantErr bool
	}{
		{
			args: args{
				s: "name",
			},
			want: []segment{
				{s: "name", wildcard: false},
			},
		},
		{
			args: args{
				s: "{name}",
			},
			want: []segment{
				{s: "name", wildcard: true},
			},
		},
		{
			args: args{
				s: "any-{repo}-any/any-{name}-any",
			},
			want: []segment{
				{s: "any-", wildcard: false},
				{s: "repo", wildcard: true},
				{s: "-any/any-", wildcard: false},
				{s: "name", wildcard: true},
				{s: "-any", wildcard: false},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSegments(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSegments() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSegments() got = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func Test_matchSegments(t *testing.T) {
	type args struct {
		segs []segment
		s    string
	}
	tests := []struct {
		name  string
		args  args
		want  map[string]string
		want1 bool
	}{
		{
			args: args{
				segs: []segment{
					{s: "name", wildcard: false},
				},
				s: "name",
			},
			want:  map[string]string{},
			want1: true,
		},
		{
			args: args{
				segs: []segment{
					{s: "name", wildcard: false},
				},
				s: "name1",
			},
			want:  map[string]string{},
			want1: false,
		},
		{
			args: args{
				segs: []segment{
					{s: "name", wildcard: true},
				},
				s: "name1",
			},
			want: map[string]string{
				"name": "name1",
			},
			want1: true,
		},
		{
			args: args{
				segs: []segment{
					{s: "any-", wildcard: false},
					{s: "name", wildcard: true},
					{s: "-any", wildcard: false},
				},
				s: "any-name-any",
			},
			want: map[string]string{
				"name": "name",
			},
			want1: true,
		},
		{
			args: args{
				segs: []segment{
					{s: "pre", wildcard: true},
					{s: "-any-", wildcard: false},
					{s: "post", wildcard: true},
				},
				s: "pre-any-post",
			},
			want: map[string]string{
				"pre":  "pre",
				"post": "post",
			},
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := matchSegments(tt.args.segs, tt.args.s)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("matchSegments() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("matchSegments() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
