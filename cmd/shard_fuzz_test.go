package cmd

import "testing"

func FuzzParseShardFlag(f *testing.F) {
	f.Add("")
	f.Add("0/1")
	f.Add("3/4")
	f.Add("4/4")
	f.Add("-1/2")
	f.Add("1/4abc")
	f.Add("a/b")
	f.Add("99999999999999999999/1")

	f.Fuzz(func(t *testing.T, shard string) {
		index, total, err := parseShardFlag(shard)
		if err != nil {
			return
		}
		if total <= 0 || index < 0 || index >= total {
			t.Fatalf("parseShardFlag(%q) = %d/%d, out of range", shard, index, total)
		}
	})
}
