package cyw43439

import (
	"errors"
	"testing"
)

func TestTinyBug(t *testing.T) {
	var nilErr error
	gotErr := bug(errors.New("iserr"))
	gotNil := bug(nilErr)
	if !gotErr {
		t.Error("gotErr")
	}
	if gotNil {
		t.Error("gotNil")
	}
}

func bug(args ...any) bool {
	for _, a := range args {
		switch a.(type) {
		case error:
			return true
		case nil:
			return false
		}
	}
	panic("misuse")
}
