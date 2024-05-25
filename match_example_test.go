package redglob_test

import (
	"fmt"

	"github.com/maolonglong/redglob"
)

func ExampleMatch() {
	fmt.Println(redglob.Match("foo", "f*"))
	// Output: true
}
