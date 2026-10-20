package controller

import "gooze.dev/pkg/gooze/internal/domain"

// Message types.
type estimationMsg struct {
	total     int
	hint      string
	paths     int
	fileStats map[string]fileStat
	err       error
}

type upcomingMsg struct {
	count int
}

type startMutationMsg struct {
	id          string
	thread      int
	kind        any
	fileHash    string
	displayPath string
}

type completedMutationMsg struct {
	id          string
	kind        any
	fileHash    string
	displayPath string
	status      string
	diff        []byte
}

type fileStat struct {
	path   string
	count  int
	cached bool
}

type concurrencyMsg struct {
	threads    int
	shardIndex int
	shards     int
}

type mutationScoreMsg struct {
	score domain.MutationScore
}

// List item types.
type fileItem struct {
	path   string
	count  int
	cached bool
}

func (f fileItem) FilterValue() string {
	return f.path
}
