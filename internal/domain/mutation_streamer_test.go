package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestMutationStreamer_Get_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: m.MutationArithmetic},
		{ID: "hash-2", Source: sources[0], Type: m.MutationBoolean},
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.NoError(t, ctx.Err())
	assert.Len(t, result, 2)
	assert.Equal(t, mutations[0].ID, result[0].ID)
	assert.Equal(t, mutations[1].ID, result[1].ID)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

func TestMutationStreamer_Get_DiscoverSourcesError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	testErr := errors.New("failed to get sources")
	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(nil, testErr)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
	mockFSAdapter.AssertExpectations(t)
}

func TestMutationStreamer_Get_GenerateMutationError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	testErr := errors.New("failed to generate mutations")
	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(nil, testErr)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

func TestMutationStreamer_Get_MultipleSources(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test1.go", Hash: "hash1"}},
		{Origin: &m.File{FullPath: "test2.go", Hash: "hash2"}},
	}

	mutations1 := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: m.MutationArithmetic},
	}
	mutations2 := []m.Mutation{
		{ID: "hash-2", Source: sources[1], Type: m.MutationBoolean},
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(mutations1, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[1], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(mutations2, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"."}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Len(t, result, 2)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

func TestMutationStreamer_Get_ContextCancelled(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Run(func(_ context.Context, _ m.Source, _ ...m.MutationType) {
		cancel() // Cancel context during mutation generation
	}).Return([]m.Mutation{}, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
	assert.Error(t, ctx.Err())
}

func TestMutationStreamer_Get_NoSources(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return([]m.Source{}, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
	mockFSAdapter.AssertExpectations(t)
}

func TestMutationStreamer_Get_NoMutations(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return([]m.Mutation{}, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

func TestMutationStreamer_Get_ThreadsZeroNormalizesToOne(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: m.MutationArithmetic},
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act - Pass threads=0, should not panic
	ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 0)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Len(t, result, 1)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

func TestMutationStreamer_Get_WithExcludePatterns(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	exclude := []string{"*_test.go", "vendor/*"}

	sources := []m.Source{
		{Origin: &m.File{FullPath: "main.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: m.MutationArithmetic},
	}

	mockFSAdapter.EXPECT().Get(ctx, []m.Path{"."}, exclude[0], exclude[1]).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, sources[0], domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Act
	ch := streamer.Get(ctx, []m.Path{"."}, exclude, 4)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Len(t, result, 1)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

// Benchmarks for memory allocation

func BenchmarkMutationStreamer_Get_SmallSet(b *testing.B) {
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := make([]m.Mutation, 10)
	for i := range mutations {
		mutations[i] = m.Mutation{ID: "hash-" + string(rune('0'+i)), Source: sources[0], Type: m.MutationArithmetic}
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, 4)
		for range ch {
		}
	}
}

func BenchmarkMutationStreamer_Get_MediumSet(b *testing.B) {
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := make([]m.Source, 10)
	for i := range sources {
		sources[i] = m.Source{Origin: &m.File{FullPath: m.Path("test" + string(rune('0'+i)) + ".go"), Hash: "hash"}}
	}

	mutations := make([]m.Mutation, 100)
	for i := range mutations {
		mutations[i] = m.Mutation{ID: "hash-" + string(rune(i)), Source: sources[0], Type: m.MutationArithmetic}
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := streamer.Get(ctx, []m.Path{"."}, nil, 8)
		for range ch {
		}
	}
}

func BenchmarkMutationStreamer_Get_LargeSet(b *testing.B) {
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := make([]m.Source, 100)
	for i := range sources {
		sources[i] = m.Source{Origin: &m.File{FullPath: m.Path("test" + string(rune(i)) + ".go"), Hash: "hash"}}
	}

	mutations := make([]m.Mutation, 1000)
	for i := range mutations {
		mutations[i] = m.Mutation{ID: "hash-" + string(rune(i)), Source: sources[0], Type: m.MutationArithmetic}
	}

	mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mutations, nil)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := streamer.Get(ctx, []m.Path{"."}, nil, 16)
		for range ch {
		}
	}
}

func BenchmarkMutationStreamer_Get_BufferSizes(b *testing.B) {
	ctx := context.Background()

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := make([]m.Mutation, 100)
	for i := range mutations {
		mutations[i] = m.Mutation{ID: "hash-" + string(rune(i)), Source: sources[0], Type: m.MutationArithmetic}
	}

	bufferSizes := []int{1, 4, 8, 16, 32}

	for _, bufSize := range bufferSizes {
		b.Run("buffer_"+string(rune('0'+bufSize)), func(b *testing.B) {
			mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
			mockMutagen := new(domainmocks.MockMutagen)

			mockFSAdapter.EXPECT().Get(ctx, mock.Anything, mock.Anything).Return(sources, nil)
			mockMutagen.EXPECT().GenerateMutation(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mutations, nil)

			streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				ch := streamer.Get(ctx, []m.Path{"test.go"}, nil, bufSize)
				for range ch {
				}
			}
		})
	}
}

// ShardMutations tests

func TestMutationStreamer_ShardMutations_EvenDistribution(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// Create input channel with 6 mutations
	inputCh := make(chan m.Mutation, 6)
	for i := 0; i < 6; i++ {
		inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh)

	// Act - shard into 3 shards
	shard0Ch := streamer.ShardMutations(ctx, inputCh, 4, 0, 3)

	var shard0 []m.Mutation
	for mutation := range shard0Ch {
		shard0 = append(shard0, mutation)
	}

	// Assert - shard 0 should get mutations 0, 3 (indices 0, 3 mod 3 == 0)
	assert.Len(t, shard0, 2)
	assert.Equal(t, "mutation-0", shard0[0].ID)
	assert.Equal(t, "mutation-3", shard0[1].ID)
}

func TestMutationStreamer_ShardMutations_AllShardsGetEqualMutations(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	totalShards := 3
	totalMutations := 9

	// Test each shard
	for shardIndex := 0; shardIndex < totalShards; shardIndex++ {
		inputCh := make(chan m.Mutation, totalMutations)
		for i := 0; i < totalMutations; i++ {
			inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
		}
		close(inputCh)

		shardCh := streamer.ShardMutations(ctx, inputCh, 4, shardIndex, totalShards)

		var shardMutations []m.Mutation
		for mutation := range shardCh {
			shardMutations = append(shardMutations, mutation)
		}

		// Each shard should get exactly 3 mutations (9 / 3)
		assert.Len(t, shardMutations, 3, "Shard %d should have 3 mutations", shardIndex)
	}
}

func TestMutationStreamer_ShardMutations_DisabledSharding(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	inputCh := make(chan m.Mutation, 5)
	for i := 0; i < 5; i++ {
		inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh)

	// Act - totalShardCount <= 0 should pass through all mutations
	ch := streamer.ShardMutations(ctx, inputCh, 4, 0, 0)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert - all mutations should pass through
	assert.Len(t, result, 5)
}

func TestMutationStreamer_ShardMutations_NegativeShardCount(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	inputCh := make(chan m.Mutation, 3)
	for i := 0; i < 3; i++ {
		inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh)

	// Act - negative totalShardCount should pass through all mutations
	ch := streamer.ShardMutations(ctx, inputCh, 4, 0, -1)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Len(t, result, 3)
}

func TestMutationStreamer_ShardMutations_ContextCancelled(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	inputCh := make(chan m.Mutation, 10)
	for i := 0; i < 10; i++ {
		inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh)

	// Cancel context immediately
	cancel()

	// Act
	ch := streamer.ShardMutations(ctx, inputCh, 4, 0, 2)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert - should get few or no mutations due to cancellation
	assert.True(t, len(result) < 10, "Should have received fewer mutations due to cancellation")
}

func TestMutationStreamer_ShardMutations_SingleShard(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	inputCh := make(chan m.Mutation, 5)
	for i := 0; i < 5; i++ {
		inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh)

	// Act - single shard should get all mutations
	ch := streamer.ShardMutations(ctx, inputCh, 4, 0, 1)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Len(t, result, 5)
}

func TestMutationStreamer_ShardMutations_LastShardGetsRemainder(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// 7 mutations with 3 shards: shard 0 gets 3, shard 1 gets 2, shard 2 gets 2
	totalMutations := 7
	totalShards := 3

	shardCounts := make([]int, totalShards)

	for shardIndex := 0; shardIndex < totalShards; shardIndex++ {
		inputCh := make(chan m.Mutation, totalMutations)
		for i := 0; i < totalMutations; i++ {
			inputCh <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
		}
		close(inputCh)

		shardCh := streamer.ShardMutations(ctx, inputCh, 4, shardIndex, totalShards)

		for range shardCh {
			shardCounts[shardIndex]++
		}
	}

	// Assert - total should equal original count
	total := 0
	for _, count := range shardCounts {
		total += count
	}
	assert.Equal(t, totalMutations, total)

	// Round-robin: indices 0,3,6 -> shard 0 (3), indices 1,4 -> shard 1 (2), indices 2,5 -> shard 2 (2)
	assert.Equal(t, 3, shardCounts[0])
	assert.Equal(t, 2, shardCounts[1])
	assert.Equal(t, 2, shardCounts[2])
}

func TestMutationStreamer_ShardMutations_EmptyInput(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	inputCh := make(chan m.Mutation)
	close(inputCh)

	// Act
	ch := streamer.ShardMutations(ctx, inputCh, 4, 0, 3)

	var result []m.Mutation
	for mutation := range ch {
		result = append(result, mutation)
	}

	// Assert
	assert.Empty(t, result)
}

func TestMutationStreamer_ShardMutations_DeterministicDistribution(t *testing.T) {
	// Arrange - run same sharding twice, should get same results
	ctx := context.Background()
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockMutagen := new(domainmocks.MockMutagen)

	streamer := domain.NewMutationStreamer(mockFSAdapter, mockMutagen)

	// First run
	inputCh1 := make(chan m.Mutation, 6)
	for i := 0; i < 6; i++ {
		inputCh1 <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh1)

	ch1 := streamer.ShardMutations(ctx, inputCh1, 4, 1, 3)
	var result1 []m.Mutation
	for mutation := range ch1 {
		result1 = append(result1, mutation)
	}

	// Second run with same input
	inputCh2 := make(chan m.Mutation, 6)
	for i := 0; i < 6; i++ {
		inputCh2 <- m.Mutation{ID: "mutation-" + string(rune('0'+i))}
	}
	close(inputCh2)

	ch2 := streamer.ShardMutations(ctx, inputCh2, 4, 1, 3)
	var result2 []m.Mutation
	for mutation := range ch2 {
		result2 = append(result2, mutation)
	}

	// Assert - both runs should produce identical results
	assert.Equal(t, len(result1), len(result2))
	for i := range result1 {
		assert.Equal(t, result1[i].ID, result2[i].ID)
	}
}
