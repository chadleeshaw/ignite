package tftp

import (
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
)

func TestNewServer(t *testing.T) {
	serveDir := "."
	server := NewServer(serveDir, slog.Default())
	if server.serveDir != serveDir {
		t.Errorf("Expected serveDir to be %s, but got %s", serveDir, server.serveDir)
	}
	if server.listener != nil {
		t.Error("Expected listener to be nil initially")
	}
	if server.tftpServer != nil {
		t.Error("Expected tftpServer to be nil initially")
	}
}

func TestServerStart(t *testing.T) {
	serveDir := "."
	server := NewServer(serveDir, slog.Default())
	server.Port = 0 // Use a dynamic port for testing

	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Check if the listener is now set
	if server.listener == nil {
		t.Fatal("Server listener should be set after Start")
	}

	// Get the dynamic port from the listener
	addr := server.listener.LocalAddr().(*net.UDPAddr)

	// Attempt to connect to the server to verify it's listening
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Errorf("Failed to connect to server: %v", err)
	} else {
		conn.Close()
	}
}

func TestReadHandler(t *testing.T) {
	serveDir := t.TempDir()
	server := NewServer(serveDir, slog.Default())

	testFile := serveDir + "/testfile.txt"
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	rf := &mockReaderFrom{}
	err = server.readHandler("testfile.txt", rf)
	if err != nil {
		t.Errorf("Read handler should not return an error: %v", err)
	}
	if string(rf.data) != "test content" {
		t.Errorf("Expected 'test content', but got '%s'", string(rf.data))
	}
	server.Stop()
}

func TestWriteHandler(t *testing.T) {
	serveDir := t.TempDir()
	server := NewServer(serveDir, slog.Default())

	wt := &mockWriterTo{data: []byte("test write content")}
	err := server.writeHandler("testwritefile.txt", wt)
	if err != nil {
		t.Errorf("Write handler returned an error: %v", err)
	}

	data, err := os.ReadFile(serveDir + "/testwritefile.txt")
	if err != nil {
		t.Fatalf("Failed to read the written file: %v", err)
	}
	if string(data) != "test write content" {
		t.Errorf("Expected 'test write content', but got '%s'", string(data))
	}
	defer os.Remove(serveDir + "/testwritefile.txt")
	server.Stop()
}

type mockReaderFrom struct {
	data []byte
}

func (m *mockReaderFrom) ReadFrom(r io.Reader) (int64, error) {
	var buf [1024]byte
	n, err := r.Read(buf[:])
	m.data = append(m.data, buf[:n]...)
	return int64(n), err
}

type mockWriterTo struct {
	data []byte
}

func (m *mockWriterTo) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(m.data)
	return int64(n), err
}
