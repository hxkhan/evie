package evie_test

import (
	"strings"
	"testing"

	"github.com/hxkhan/evie/vm"
)

// the public operators behave like the script's, with the script's errors
func TestPublicOperators(t *testing.T) {
	n := vm.BoxNumber
	s := vm.BoxString

	if v, err := n(2).Add(n(3)); err != nil || v.String() != "5" {
		t.Fatalf("2 + 3 = %v, %v", v, err)
	}
	if v, err := s("ab").Add(s("cd")); err != nil || v.String() != "abcd" {
		t.Fatalf(`"ab" + "cd" = %v, %v`, v, err)
	}
	if v, err := n(7).Mod(n(4)); err != nil || v.String() != "3" {
		t.Fatalf("7 %% 4 = %v, %v", v, err)
	}
	if ok, err := n(1).LessThan(n(2)); err != nil || !ok {
		t.Fatalf("1 < 2 = %v, %v", ok, err)
	}
	if ok, err := n(2).GreaterThanOrEqualTo(n(2)); err != nil || !ok {
		t.Fatalf("2 >= 2 = %v, %v", ok, err)
	}
	if _, err := n(1).Add(s("x")); err == nil || !strings.Contains(err.Error(), "cannot apply '+' to a number and a string") {
		t.Fatalf("1 + \"x\" gave %v", err)
	}
	if _, err := s("a").LessThan(s("b")); err == nil || !strings.Contains(err.Error(), "cannot apply '<'") {
		t.Fatalf(`"a" < "b" gave %v`, err)
	}
	if !n(1).Equals(n(1)) || n(0).IsTruthy() || !s("x").IsTruthy() {
		t.Fatal("Equals or IsTruthy disagree with the language")
	}
}
