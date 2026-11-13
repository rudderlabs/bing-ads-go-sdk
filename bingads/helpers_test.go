package bingads

import (
	"io"
	"os"
	"testing"
)

func enableDebug(t *testing.T) {
	t.Helper()
	old := debug
	debug = "1"
	t.Cleanup(func() { debug = old })
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = old }()
	run()
	_ = writer.Close()
	out, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
