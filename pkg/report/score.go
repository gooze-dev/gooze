package report

// Summary counts mutation results by status.
type Summary struct {
	Total      int
	Killed     int
	Survived   int
	Skipped    int
	Timeout    int
	Error      int
	NotCovered int
}

// Add counts one mutation result. Unknown statuses count only towards Total.
func (s *Summary) Add(status TestStatus) {
	s.Total++

	switch status {
	case Killed:
		s.Killed++
	case Survived:
		s.Survived++
	case Skipped:
		s.Skipped++
	case Timeout:
		s.Timeout++
	case Error:
		s.Error++
	case NotCovered:
		s.NotCovered++
	}
}

// Merge adds the counts of other to s.
func (s *Summary) Merge(other Summary) {
	s.Total += other.Total
	s.Killed += other.Killed
	s.Survived += other.Survived
	s.Skipped += other.Skipped
	s.Timeout += other.Timeout
	s.Error += other.Error
	s.NotCovered += other.NotCovered
}

// Scored returns the number of mutations that count towards the score: all of
// them except Error, since a mutation that does not compile is not viable.
func (s Summary) Scored() int {
	return s.Total - s.Error
}

// Score returns the mutation score of s, see Score.
func (s Summary) Score() float64 {
	return Score(s.Killed, s.Scored())
}

// Summarize counts the mutation results of r.
func Summarize(r Report) Summary {
	var s Summary

	for _, result := range r.Result {
		for _, mutation := range result.Mutations {
			s.Add(mutation.Status)
		}
	}

	return s
}

// Score returns killed/scored in the 0..1 range, where scored is the number of
// mutations excluding Error ones (see Summary.Scored). Survived, Skipped,
// Timeout and NotCovered mutations count as not killed. An empty set, or a non-positive scored count, scores 1.
func Score(killed, scored int) float64 {
	if scored <= 0 {
		return 1.0
	}

	return float64(killed) / float64(scored)
}
