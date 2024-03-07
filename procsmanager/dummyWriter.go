package procsmanager

// dummy io.Writer implementation

type DummyWriter struct {
}

func (d *DummyWriter) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (d *DummyWriter) Close() error {
	return nil
}

func NewDummyWriter() *DummyWriter {
	return &DummyWriter{}
}
