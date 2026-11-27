package main

import (
	"reflect"
	"testing"
)

func TestRepeatableFlagsKeepCommasInsideValues(t *testing.T) {
	cmd := newCreateCmd()
	if err := cmd.ParseFlags([]string{"--copy-root-file", "a,b", "--copy-root-file", "c", "--package", "x,y"}); err != nil {
		t.Fatal(err)
	}
	files, _ := cmd.Flags().GetStringArray("copy-root-file")
	if !reflect.DeepEqual(files, []string{"a,b", "c"}) {
		t.Fatalf("copy-root-file = %q", files)
	}
	packages, _ := cmd.Flags().GetStringArray("package")
	if !reflect.DeepEqual(packages, []string{"x,y"}) {
		t.Fatalf("package = %q", packages)
	}
}
