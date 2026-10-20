package domain

import "testing"

func FuzzParseIgnoreDirective(f *testing.F) {
	f.Add("//gooze:ignore")
	f.Add("// gooze:ignore arithmetic, numbers")
	f.Add("/* gooze:ignore , , */")
	f.Add("//gooze:ignoreX")
	f.Add("")

	f.Fuzz(func(t *testing.T, comment string) {
		rule, ok := parseIgnoreDirective(comment)
		if !ok {
			if rule.all || len(rule.names) != 0 {
				t.Fatalf("non-directive %q produced a rule %+v", comment, rule)
			}
			return
		}
		if rule.all && len(rule.names) != 0 {
			t.Fatalf("directive %q is both all and named: %+v", comment, rule)
		}
		if !rule.all && len(rule.names) == 0 {
			t.Fatalf("directive %q ignores nothing", comment)
		}
		for name := range rule.names {
			if name == "" {
				t.Fatalf("directive %q produced an empty name", comment)
			}
		}
	})
}
