package services

import (
	"errors"
	"fmt"
	"testing"
)

func TestFirstPublicCode_nested(t *testing.T) {
	inner := ErrCode("inner_code", errors.New("boom"))
	outer := ErrCode("outer_code", fmt.Errorf("wrap: %w", inner))
	if got := FirstPublicCode(outer); got != "inner_code" {
		t.Fatalf("FirstPublicCode: got %q", got)
	}
	if got := PublicCode(outer); got != "outer_code" {
		t.Fatalf("PublicCode: got %q", got)
	}
}

func TestFirstPublicCode_plain(t *testing.T) {
	if got := FirstPublicCode(errors.New("plain")); got != "internal_error" {
		t.Fatalf("got %q", got)
	}
}
