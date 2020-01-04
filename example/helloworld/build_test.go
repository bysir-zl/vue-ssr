package main

import (
	"go.zhuzi.me/go/util"
	"testing"
)

func TestShouldLookInterface(t *testing.T) {
	var i interface{}
	util.CopyObj([]rune("q333"), &i)
	desc, _ := shouldLookInterface(i, "length")
	if desc != 4 {
		t.Fatalf("%v %v", i, desc)
	}

	util.CopyObj("q333", &i)
	desc, _ = shouldLookInterface(i, "length")
	if desc != 4 {
		t.Fatalf("%v %v", i, desc)
	}

	util.CopyObj([]int{2,3}, &i)
	desc, _ = shouldLookInterface(i, "length")
	if desc != 2 {
		t.Fatalf("%v %v", i, desc)
	}

	util.CopyObj([]int{2,3}, &i)
	desc, _ = shouldLookInterface(i, "0")
	if desc.(float64) != 2 {
		t.Fatalf("%v %v", i, desc)
	}

	t.Log("ok")
}
