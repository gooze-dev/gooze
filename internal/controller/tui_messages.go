package controller

// Message types.
type estimationMsg struct {
	total     int
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
	kind        interface{}
	fileHash    string
	displayPath string
}

type completedMutationMsg struct {
	id          string
	kind        interface{}
	fileHash    string
	displayPath string
	status      string
	diff        []byte
}

type fileStat struct {
	path  string
	count int
}

type concurrencyMsg struct {
	threads    int
	shardIndex int
	shards     int
}

// List item types.
type fileItem struct {
	path  string
	count int
}

func (f fileItem) FilterValue() string {
	return f.path
}
