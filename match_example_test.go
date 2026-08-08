package redglob_test

import (
	"fmt"

	"github.com/maolonglong/redglob"
)

func ExampleMatch() {
	fmt.Println(redglob.Match("foo", "f*"))
	// Output: true
}

func ExampleMatchFold() {
	fmt.Println(redglob.MatchFold("Foo", "f*"))
	// Output: true
}

func ExampleCompile() {
	pattern := redglob.Compile("user:[0-9]*")
	fmt.Println(pattern.Match("user:42"))
	fmt.Println(pattern.Match("admin:42"))
	// Output:
	// true
	// false
}
