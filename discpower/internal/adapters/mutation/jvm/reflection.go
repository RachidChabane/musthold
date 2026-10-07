package jvm

import (
	"slices"
	"strings"
)

// reflectionNames are the identifiers whose use makes a unit unsafe for
// sentinels: they observe or build code and members by name.
var reflectionNames = []string{
	"getClass", "forName", "getDeclaredFields", "getDeclaredMethods", "getDeclaredMethod",
	"getMethod", "getMethods", "getFields", "getField", "newInstance", "invoke",
	"MethodHandle", "MethodHandles", "Proxy", "javaClass", "KClass",
}

// reflectionMarkers are the texts that mark a unit as reflective.
var reflectionMarkers = []string{"java.lang.reflect", "kotlin.reflect", "::class"}

// usesReflection reports whether the unit names a reflection API.
func (p *parsed) usesReflection(u unit) bool {
	for name := range p.leafNames(u.node) {
		if slices.Contains(reflectionNames, name) {
			return true
		}
	}
	for _, marker := range reflectionMarkers {
		if strings.Contains(u.text, marker) {
			return true
		}
	}
	return false
}
