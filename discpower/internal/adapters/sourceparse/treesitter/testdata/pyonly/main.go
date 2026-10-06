package main

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	lang := grammars.PythonLanguage()
	src := []byte("class A:\n    def m(self):\n        return 1\n")
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		panic(err)
	}
	root := tree.RootNode()
	cls := root.NamedChild(0)
	body := cls.ChildByFieldName("body", lang)
	fn := body.NamedChild(0)
	name := fn.ChildByFieldName("name", lang)
	fmt.Println(fn.Type(lang), name.Text(src), fn.StartByte(), fn.EndByte())
}
