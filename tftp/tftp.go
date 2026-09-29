package tftp

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	v3 "github.com/pin/tftp/v3"
)

// Server manages the TFTP server, handling file read and write operations.
type Server struct {
	serveDir    string
	absServeDir string

	mu         sync.Mutex
	listener   *net.UDPConn
	tftpServer *v3.Server
	wg         sync.WaitGroup
	running    bool
}

// NewServer creates and returns a new TFTP server instance with the specified directory for serving files.
func NewServer(serveDir string) *Server {
	abs, err := filepath.Abs(serveDir)
	if err != nil {
		abs = serveDir
	}
	return &Server{
		serveDir:    serveDir,
		absServeDir: abs,
	}
}

// resolvePath maps a client-supplied TFTP filename to a path inside the
// serve directory. Absolute paths and any path that escapes the serve
// directory (via ".." segments or otherwise) are rejected.
func (s *Server) resolvePath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("tftp: empty filename")
	}
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("tftp: absolute path %q not allowed", filename)
	}
	cleaned := filepath.Clean(filename)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("tftp: path %q escapes serve directory", filename)
	}
	full := filepath.Join(s.absServeDir, cleaned)
	rel, err := filepath.Rel(s.absServeDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("tftp: path %q escapes serve directory", filename)
	}
	return full, nil
}

// Start initiates the TFTP server, listening for incoming connections on port 69.
// The UDP socket is bound before Start reports success. It returns an error if
// the server is already running.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("tftp: server already running")
	}

	listener, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 69})
	if err != nil {
		return fmt.Errorf("tftp: failed to bind UDP port 69: %w", err)
	}

	tftpServer := v3.NewServer(s.readHandler, s.writeHandler)

	s.listener = listener
	s.tftpServer = tftpServer
	s.running = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := tftpServer.Serve(listener); err != nil {
			log.Printf("TFTP server stopped with error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the TFTP server: it stops accepting new
// requests, waits for outstanding transfers to finish, then waits for the
// serve goroutine to exit. It is safe to call on a server that was never
// started.
func (s *Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	tftpServer := s.tftpServer
	listener := s.listener
	s.tftpServer = nil
	s.listener = nil
	s.mu.Unlock()

	if tftpServer != nil {
		// Shutdown blocks until outstanding transfers complete.
		tftpServer.Shutdown()
	}
	if listener != nil {
		_ = listener.Close()
	}
	s.wg.Wait()
}

// readHandler serves file read requests by opening and reading from the specified file in the server's directory.
func (s *Server) readHandler(filename string, rf io.ReaderFrom) error {
	filePath, err := s.resolvePath(filename)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = rf.ReadFrom(file)
	return err
}

// writeHandler handles file write requests by creating a new file or overwriting an existing one in the server's directory.
func (s *Server) writeHandler(filename string, wt io.WriterTo) error {
	log.Printf("Write request for %s", filename)

	filePath, err := s.resolvePath(filename)
	if err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = wt.WriteTo(file)
	return err
}
