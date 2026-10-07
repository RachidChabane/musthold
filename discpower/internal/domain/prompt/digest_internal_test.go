package prompt

import "testing"

const (
	baseSystem   = "system text"
	baseUser     = "user text"
	baseSchema   = `{"type":"object"}`
	baseInstruct = "instructions text"
)

func TestTemplateDigestFollowsTheTemplate(t *testing.T) {
	base := build(baseSystem, baseUser, baseSchema, nil, baseInstruct).Prompt.TemplateDigest
	cases := []struct {
		name    string
		built   Built
		changed bool
	}{
		{"the same template", build(baseSystem, baseUser, baseSchema, nil, baseInstruct), false},
		{"another user text", build(baseSystem, "other user text", baseSchema, nil, baseInstruct), false},
		{"another closed payload", build(baseSystem, baseUser, baseSchema, []byte(`{"state":"x"}`), baseInstruct), false},
		{"another system text", build("other system text", baseUser, baseSchema, nil, baseInstruct), true},
		{"another schema", build(baseSystem, baseUser, `{"type":"array"}`, nil, baseInstruct), true},
		{"other instructions", build(baseSystem, baseUser, baseSchema, nil, "other instructions"), true},
		{"an extra template part", build(baseSystem, baseUser, baseSchema, nil, baseInstruct, "more"), true},
		{"parts moved across the boundary between them", build(baseSystem+partSeparator, baseUser, baseSchema[:1], nil, baseSchema[1:]+baseInstruct), true},
	}
	for _, tc := range cases {
		t.Run(tc.name+" [verifies SP-173~1]", func(t *testing.T) {
			if got := tc.built.Prompt.TemplateDigest != base; got != tc.changed {
				t.Errorf("digest changed = %v, want %v", got, tc.changed)
			}
		})
	}
	t.Run("the built prompt exposes its schema and closed payload unchanged [verifies SP-173~1]", func(t *testing.T) {
		built := build(baseSystem, baseUser, baseSchema, []byte(`{"a":1}`), baseInstruct)
		if string(built.Schema) != baseSchema || string(built.Closed) != `{"a":1}` || built.Prompt.System != baseSystem || built.Prompt.User != baseUser {
			t.Errorf("built %+v", built)
		}
	})
}

func TestEmbeddedFilesAreAllPresent(t *testing.T) {
	for _, name := range []string{
		generateSystem, generateInstructions, judgeSystem, validityInstructions, equivalentInstruction,
		screenSystem, screenInstructions, generateSchema, validitySchema, equivalentSchema, closedSchema,
	} {
		t.Run(name+" is embedded and not empty [verifies SP-173~1]", func(t *testing.T) {
			if embedded(name) == "" {
				t.Error("empty file")
			}
		})
	}
	t.Run("a missing embedded file panics [verifies SP-173~1]", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("no panic")
			}
		}()
		embedded("templates/missing.txt")
	})
}
