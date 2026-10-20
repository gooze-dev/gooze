package domain

import (
	"strings"
	"testing"
)

func FuzzParseCoverage(f *testing.F) {
	f.Add([]byte("mode: set\nexample.com/m/a.go:1.2,3.4 2 1\nexample.com/m/a.go:5.1,6.1 1 0\n"))
	f.Add([]byte("garbage\n:::\na.go:1.1\n"))
	f.Add([]byte("C:\\proj\\a.go:1.1,2.1 1 1\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, content []byte) {
		idx, err := ParseCoverage(content)
		if err != nil {
			t.Fatalf("ParseCoverage returned error: %v", err)
		}
		if idx == nil {
			t.Fatal("ParseCoverage returned nil index without error")
		}
		for path := range idx.files {
			if strings.TrimSpace(path) == "" {
				t.Fatalf("indexed empty path from %q", content)
			}
			_ = idx.Covers(path, 1)
		}
		_ = idx.Covers("a.go", 0)
		_ = idx.Covers("", -1)
	})
}
