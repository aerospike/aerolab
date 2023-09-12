package ingest

type logStream struct{}

type logStreamOutput struct {
	Data    map[string]interface{}
	Line    string
	Error   error
	SetName string
}

func newLogStream() *logStream {
	return &logStream{}
}

func (s *logStream) Close() []*logStreamOutput {
	return nil
}

func (s *logStream) Process(line string) ([]*logStreamOutput, error) {
	return nil, nil
}
