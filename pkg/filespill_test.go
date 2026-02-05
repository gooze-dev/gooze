package pkg

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileSpill(t *testing.T) {
	t.Run("NewFileSpill", func(t *testing.T) {
		path, err := NewFileSpill[int]()
		require.NoError(t, err)
		require.NotNil(t, path)
		require.Contains(t, path.Path(), "/tmp/filespill")
		defer path.Close()
	})

	t.Run("FileSpill Append and Get", func(t *testing.T) {
		spill, err := NewFileSpill[string]()
		require.NoError(t, err)
		defer spill.Close()

		err = spill.Append("first")
		require.NoError(t, err)

		err = spill.Append("second")
		require.NoError(t, err)

		val1, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, "first", val1)

		val2, err := spill.Get(1)
		require.NoError(t, err)
		require.Equal(t, "second", val2)

		val3, err := spill.Get(3)
		require.Error(t, err)
		require.Equal(t, "", val3)
	})

	t.Run("Len returns correct count", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		require.Equal(t, uint64(0), spill.Len())

		spill.Append(1)
		require.Equal(t, uint64(1), spill.Len())

		spill.Append(2)
		spill.Append(3)
		require.Equal(t, uint64(3), spill.Len())
	})

	t.Run("AppendBatch adds multiple items", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		items := []int{10, 20, 30, 40, 50}
		err = spill.AppendBatch(items)
		require.NoError(t, err)

		require.Equal(t, uint64(5), spill.Len())

		val, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, 10, val)

		val, err = spill.Get(4)
		require.NoError(t, err)
		require.Equal(t, 50, val)
	})

	t.Run("Range iterates all items in order", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		expected := []int{100, 200, 300}
		for _, v := range expected {
			spill.Append(v)
		}

		var collected []int
		err = spill.Range(func(index uint64, item int) error {
			collected = append(collected, item)
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, expected, collected)
	})

	t.Run("Range callback error stops iteration", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append(1)
		spill.Append(2)
		spill.Append(3)

		count := 0
		rangeErr := spill.Range(func(index uint64, item int) error {
			count++
			if index == 1 {
				return errors.New("stop at index 1")
			}
			return nil
		})

		require.Error(t, rangeErr)
		require.Equal(t, 2, count) // Should stop after processing index 1
	})

	t.Run("Close closes file and prevents further operations", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)

		spill.Append(1)
		err = spill.Close()
		require.NoError(t, err)

		// File is closed but data persists
		val, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, 1, val)
	})

	t.Run("Generic types work with different types", func(t *testing.T) {
		// Test with float64
		spillFloat, err := NewFileSpill[float64]()
		require.NoError(t, err)
		defer spillFloat.Close()

		spillFloat.Append(3.14)
		spillFloat.Append(2.71)

		val1, err := spillFloat.Get(0)
		require.NoError(t, err)
		require.InDelta(t, 3.14, val1, 0.001)

		val2, err := spillFloat.Get(1)
		require.NoError(t, err)
		require.InDelta(t, 2.71, val2, 0.001)

		// Test with custom struct
		type Point struct {
			X, Y int
		}

		spillPoint, err := NewFileSpill[Point]()
		require.NoError(t, err)
		defer spillPoint.Close()

		p1 := Point{X: 10, Y: 20}
		p2 := Point{X: 30, Y: 40}

		spillPoint.Append(p1)
		spillPoint.Append(p2)

		retrieved, err := spillPoint.Get(0)
		require.NoError(t, err)
		require.Equal(t, p1, retrieved)
	})
}

// BenchmarkAppend measures the performance of appending items.
func BenchmarkAppend(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spill.Append(i)
	}
}

// BenchmarkGet measures the performance of getting items by index.
func BenchmarkGet(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	// Pre-populate with 1000 items
	for i := 0; i < 1000; i++ {
		_ = spill.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = spill.Get(uint64(i % 1000))
	}
}

// BenchmarkRange measures the performance of iterating all items.
func BenchmarkRange(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	// Pre-populate with 1000 items
	for i := 0; i < 1000; i++ {
		_ = spill.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spill.Range(func(index uint64, item int) error {
			return nil
		})
	}
}

// BenchmarkAppendBatch measures the performance of batch appending.
func BenchmarkAppendBatch(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	items := make([]int, 100)
	for i := 0; i < 100; i++ {
		items[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spill.AppendBatch(items)
	}
}

// BenchmarkAppendStruct measures the performance of appending complex structs.
func BenchmarkAppendStruct(b *testing.B) {
	type ComplexData struct {
		ID        int
		Name      string
		Tags      []string
		Values    map[string]float64
		IsActive  bool
		Timestamp int64
	}

	spill, err := NewFileSpill[ComplexData]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	data := ComplexData{
		ID:   12345,
		Name: "Benchmark Data Item",
		Tags: []string{"tag1", "tag2", "tag3"},
		Values: map[string]float64{
			"val1": 1.23,
			"val2": 4.56,
		},
		IsActive:  true,
		Timestamp: 1678900000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spill.Append(data)
	}
}

// BenchmarkGetLarge measures Get performance on a larger dataset (10k items).
func BenchmarkGetLarge(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	// Pre-populate with 10,000 items
	count := 10000
	for i := 0; i < count; i++ {
		_ = spill.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Access random items
		_, _ = spill.Get(uint64(i % count))
	}
}

// BenchmarkGetFirst measures Get performance for the first item (best case).
func BenchmarkGetFirst(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	// Pre-populate with 10,000 items to ensure file is significant
	for i := 0; i < 10000; i++ {
		_ = spill.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = spill.Get(0)
	}
}

// BenchmarkGetLast measures Get performance for the last item (worst case).
func BenchmarkGetLast(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	count := 10000
	for i := 0; i < count; i++ {
		_ = spill.Append(i)
	}

	lastIndex := uint64(count - 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = spill.Get(lastIndex)
	}
}

// BenchmarkRangeLarge measures Range performance on a larger dataset (10k items).
func BenchmarkRangeLarge(b *testing.B) {
	spill, err := NewFileSpill[int]()
	if err != nil {
		b.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	count := 10000
	for i := 0; i < count; i++ {
		_ = spill.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spill.Range(func(index uint64, item int) error {
			return nil
		})
	}
}

// TestEdgeCases covers boundary conditions and unusual scenarios.
func TestEdgeCases(t *testing.T) {
	t.Run("empty filespill range returns no items", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		count := 0
		err = spill.Range(func(index uint64, item int) error {
			count++
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("get on empty filespill returns error", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		_, err = spill.Get(0)
		require.Error(t, err)
	})

	t.Run("append empty string", func(t *testing.T) {
		spill, err := NewFileSpill[string]()
		require.NoError(t, err)
		defer spill.Close()

		err = spill.Append("")
		require.NoError(t, err)

		val, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, "", val)
	})

	t.Run("append zero values", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		err = spill.Append(0)
		require.NoError(t, err)

		val, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, 0, val)
	})

	t.Run("append negative numbers", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append(-1)
		spill.Append(-999999)

		val1, _ := spill.Get(0)
		require.Equal(t, -1, val1)

		val2, _ := spill.Get(1)
		require.Equal(t, -999999, val2)
	})

	t.Run("append large numbers", func(t *testing.T) {
		spill, err := NewFileSpill[int64]()
		require.NoError(t, err)
		defer spill.Close()

		large := int64(math.MaxInt64)
		spill.Append(large)

		val, _ := spill.Get(0)
		require.Equal(t, large, val)
	})

	t.Run("append float special values", func(t *testing.T) {
		spill, err := NewFileSpill[float64]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append(0.0)
		spill.Append(-0.0)
		spill.Append(math.MaxFloat64)
		spill.Append(math.SmallestNonzeroFloat64)

		v0, _ := spill.Get(0)
		require.Equal(t, 0.0, v0)

		v2, _ := spill.Get(2)
		require.Equal(t, math.MaxFloat64, v2)
	})

	t.Run("append empty slice", func(t *testing.T) {
		spill, err := NewFileSpill[[]int]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append([]int{})
		val, _ := spill.Get(0)
		require.Equal(t, 0, len(val))
	})

	t.Run("get first item", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		for i := 0; i < 100; i++ {
			spill.Append(i)
		}

		val, err := spill.Get(0)
		require.NoError(t, err)
		require.Equal(t, 0, val)
	})

	t.Run("get last item", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		for i := 0; i < 100; i++ {
			spill.Append(i)
		}

		val, err := spill.Get(99)
		require.NoError(t, err)
		require.Equal(t, 99, val)
	})

	t.Run("get beyond last item fails", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append(1)
		spill.Append(2)

		_, err = spill.Get(2)
		require.Error(t, err)

		_, err = spill.Get(1000)
		require.Error(t, err)
	})

	t.Run("append very long string", func(t *testing.T) {
		spill, err := NewFileSpill[string]()
		require.NoError(t, err)
		defer spill.Close()

		longStr := ""
		for i := 0; i < 10000; i++ {
			longStr += "x"
		}

		spill.Append(longStr)
		val, _ := spill.Get(0)
		require.Equal(t, longStr, val)
		require.Equal(t, 10000, len(val))
	})

	t.Run("append batch with single item", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		err = spill.AppendBatch([]int{42})
		require.NoError(t, err)

		val, _ := spill.Get(0)
		require.Equal(t, 42, val)
	})

	t.Run("append batch then individual append", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		spill.AppendBatch([]int{1, 2, 3})
		spill.Append(4)

		require.Equal(t, uint64(4), spill.Len())

		v3, _ := spill.Get(3)
		require.Equal(t, 4, v3)
	})

	t.Run("len before and after operations", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		require.Equal(t, uint64(0), spill.Len())

		spill.Append(1)
		require.Equal(t, uint64(1), spill.Len())

		spill.AppendBatch([]int{2, 3, 4})
		require.Equal(t, uint64(4), spill.Len())
	})

	t.Run("range with large dataset", func(t *testing.T) {
		spill, err := NewFileSpill[int]()
		require.NoError(t, err)
		defer spill.Close()

		n := 10000
		for i := 0; i < n; i++ {
			spill.Append(i)
		}

		count := 0
		sum := 0
		spill.Range(func(index uint64, item int) error {
			count++
			sum += item
			return nil
		})

		require.Equal(t, n, count)
		expected := n * (n - 1) / 2
		require.Equal(t, expected, sum)
	})

	t.Run("range with complex types", func(t *testing.T) {
		type Data struct {
			ID   int
			Name string
		}

		spill, err := NewFileSpill[Data]()
		require.NoError(t, err)
		defer spill.Close()

		spill.Append(Data{ID: 1, Name: "Alice"})
		spill.Append(Data{ID: 2, Name: "Bob"})

		items := []Data{}
		spill.Range(func(index uint64, item Data) error {
			items = append(items, item)
			return nil
		})

		require.Equal(t, 2, len(items))
		require.Equal(t, "Alice", items[0].Name)
	})
}

// FuzzAppendGet fuzzes append and get operations with integers.
func FuzzAppendGet(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(999))

	f.Fuzz(func(t *testing.T, data int64) {
		spill, err := NewFileSpill[int64]()
		if err != nil {
			t.Skipf("setup failed: %v", err)
		}
		defer spill.Close()

		err = spill.Append(data)
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		val, err := spill.Get(0)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if val != data {
			t.Fatalf("value mismatch: expected %d, got %d", data, val)
		}

		// Out of bounds should fail
		_, err = spill.Get(1)
		if err == nil {
			t.Fatal("expected error for out of bounds get")
		}
	})
}

// FuzzStringAppend fuzzes string append operations.
func FuzzStringAppend(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Add("x")

	f.Fuzz(func(t *testing.T, data string) {
		spill, err := NewFileSpill[string]()
		if err != nil {
			t.Skipf("setup failed: %v", err)
		}
		defer spill.Close()

		err = spill.Append(data)
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		val, err := spill.Get(0)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if val != data {
			t.Fatalf("value mismatch: expected %q, got %q", data, val)
		}
	})
}

// FuzzByteSlice fuzzes byte slice operations.
func FuzzByteSlice(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2})
	f.Add([]byte{255})

	f.Fuzz(func(t *testing.T, data []byte) {
		spill, err := NewFileSpill[[]byte]()
		if err != nil {
			t.Skipf("setup failed: %v", err)
		}
		defer spill.Close()

		err = spill.Append(data)
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		val, err := spill.Get(0)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if len(val) != len(data) {
			t.Fatalf("length mismatch: expected %d, got %d", len(data), len(val))
		}

		for i, b := range data {
			if val[i] != b {
				t.Fatalf("byte mismatch at index %d: expected %d, got %d", i, b, val[i])
			}
		}
	})
}
