// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package to

import "testing"

func TestPtr(t *testing.T) {
	v := "test"
	if *Ptr(v) != v {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(v), v)
	}

	v = ""
	if *Ptr(v) != v {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(v), v)
	}

	b := true
	if *Ptr(b) != b {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(b), b)
	}

	b = false
	if *Ptr(b) != b {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(b), b)
	}

	var i int32 = 10
	if *Ptr(i) != i {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(i), i)
	}

	i = 0
	if *Ptr(i) != i {
		t.Fatalf("to: Ptr() returned wrong value(%v), expected %v", *Ptr(i), i)
	}
}

func TestVal(t *testing.T) {
	v := ""
	if Val(&v) != v {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", Val(&v), v)
	}

	var pv *string
	if Val(pv) != "" {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", "", Val(pv))
	}

	b := true
	if Val(&b) != b {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", Val(&b), b)
	}

	var pb *bool
	if Val(pb) != false {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", Val(pb), false)
	}

	var i int32 = 10
	if Val(&i) != i {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", Val(&i), i)
	}

	var pi *int32
	if Val(pi) != int32(0) {
		t.Fatalf("to: Val() returned wrong value(%v), expected %v", Val(pi), 0)
	}
}
