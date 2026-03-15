package model

// EventMsg carries a single event emitted during workflow execution.
type EventMsg struct {
	Kind string
	Path Path
	Err  error
	Source Source
	Mutation Mutation
	Result map[string]struct{
		Count int
		Source Source
	}
	
}

// Events is the interface used to emit and listen to workflow progress.
// Chan() returns a read-only channel that listeners range over.
type Events interface {
	Chan() <-chan EventMsg
	Close()
	Error(err error)

	StartScanningPaths()
	ScanningPath(path Path)
	FinishScanningPaths()
	PairFound(source Source)

	StartGeneratingMutations()
	GeneratingMutationsFor(source Source)
	FinishGeneratingMutations()

	StartEstimating()
	Estimating(mutation Mutation)
	FinishEstimating()
	ShowEstimationResult(result map[string]struct{
		Count int
		Source Source
	})
}

