package main

import (
	"fmt"

	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	fmt.Println(observe())
}

func observe() (outcome string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = fmt.Sprintf("panic: %v", recovered)
		}
	}()
	if grammars.JavaLanguage() == nil {
		return "nil"
	}
	return "ok"
}
