package youtube

import (
	"embed"
	"strings"
)

// Embedded page programs. They live as real .js files so they can be linted with node --check and read
// without escaping (plan §4.3); they are wrapped in the standard prelude by Compose before being sent.
//
//go:embed scripts/*.js
var scriptFS embed.FS

// Script returns an embedded page program by name. A missing name is a build-time bug rather than a
// runtime one, so the scripts are also enumerated by TestEveryYoutubeScriptIsEmbeddedAndParseable.
func Script(name string) string {
	b, err := scriptFS.ReadFile("scripts/" + name)
	if err != nil {
		panic("youtube: missing embedded script " + name + ": " + err.Error())
	}
	return string(b)
}

// Scripts lists the embedded program names, for the lint gate.
func Scripts() []string {
	entries, _ := scriptFS.ReadDir("scripts")
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// ScriptSource returns the raw source of an embedded program, for the lint gate.
func ScriptSource(name string) string { return Script(name) }

// ComposeProgram builds the payload a script is run with.
func ComposeProgram(name string, args map[string]any) (string, error) {
	if !strings.HasSuffix(name, ".js") {
		return "", errScriptName
	}
	return Compose(Script(name), args), nil
}

type scriptErr string

func (s scriptErr) Error() string { return string(s) }

const errScriptName = scriptErr("youtube: embedded script names must end in .js")
