package domain

import m "gooze.dev/pkg/gooze/internal/model"

type events struct {
	ch chan m.EventMsg
}

func NewEvents() m.Events {
	return &events{ch: make(chan m.EventMsg, 4)}
}

func (e *events) Chan() <-chan m.EventMsg { return e.ch }
func (e *events) Close()                { close(e.ch) }
func (e *events) Error(err error)       { e.ch <- m.EventMsg{Kind: "error", Err: err} }	

func (e *events) StartScanningPaths()        { e.ch <- m.EventMsg{Kind: "start-scan"} }
func (e *events) ScanningPath(path m.Path)   { e.ch <- m.EventMsg{Kind: "scan", Path: path} }
func (e *events) FinishScanningPaths()       { e.ch <- m.EventMsg{Kind: "finish-scan"} }
func (e *events) PairFound(source m.Source)      { e.ch <- m.EventMsg{Kind: "pair-found", Source: source} }

func (e *events) StartGeneratingMutations() { e.ch <- m.EventMsg{Kind: "start-generate"} }
func (e *events) GeneratingMutationsFor(source m.Source) { e.ch <- m.EventMsg{Kind: "generating-mutations", Source: source} }
func (e *events) FinishGeneratingMutations() { e.ch <- m.EventMsg{Kind: "finish-generate"} }

func (e *events) StartEstimating()           { e.ch <- m.EventMsg{Kind: "start-estimate"} }
func (e *events) Estimating(mutation m.Mutation) { e.ch <- m.EventMsg{Kind: "estimate", Mutation: mutation} }
func (e *events) FinishEstimating() { e.ch <- m.EventMsg{Kind: "finish-estimate"} }
func (e *events) ShowEstimationResult(result map[string]struct{ Count int; Source m.Source }) { e.ch <- m.EventMsg{Kind: "show-estimation-result", Result: result} }