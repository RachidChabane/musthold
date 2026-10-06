package main

import (
	"fmt"

	"github.com/RachidChabane/spec-proof/internal/adapters/sourceparse/treesitter"
)

type sample struct {
	language string
	path     string
	source   string
}

var samples = []sample{
	{"python", "a.py", "class A:\n    def m(self):\n        return 1\n\ndef f(x):\n    return x\n"},
	{"java", "A.java", "class A { A() {} int m(int x) { return x; } }\n"},
	{"kotlin", "A.kt", "class A { fun m(x: Int): Int { return x } }\nfun top() {}\n"},
	{"typescript", "a.ts", "class A { m(x: number): number { return x } }\nfunction f(): void {}\n"},
	{"tsx", "a.tsx", "class A { render() { return <div/> } }\nfunction View() { return <span/> }\n"},
	{"javascript", "a.js", "class A { m(x) { return x } }\nfunction f() {}\n"},
}

func main() {
	locator := treesitter.NewLocator()
	for _, s := range samples {
		definitions, err := locator.Locate(s.path, []byte(s.source))
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s %d\n", s.language, len(definitions))
	}
}
