package tftp

import (
	"io"
	"log"
	"net"
	"os"
	"time"

	v3 "github.com/pin/tftp/v3"
)

// Server manages the TFTP server, handling file read and write operations.
type Server struct {
	serveDir   string
	listener   *net.UDPConn
	tftpServer *v3.Server
}

// NewServer creates and returns a new TFTP server instance with the specified directory for serving files.
func NewServer(serveDir string) *Server {
	return &Server{
		serveDir: serveDir,
	}
}

// Start initiates the TFTP server, listening for incoming connections on port 69.
func (s *Server) Start() error {
	var err error
	s.listener, err = net.ListenUDP("udp4", &net.UDPAddr{Port: 69})
	if err != nil {
		return err
	}

	s.tftpServer = v3.NewServer(s.readHandler, s.writeHandler)

	errChan := make(chan error, 1)
	go func() {
		err := s.tftpServer.Serve(s.listener)
		if err != nil {
			log.Printf("Server stopped with error: %v", err)
		}
		errChan <- err
		s.tftpServer.Shutdown()
	}()

	select {
	case err := <-errChan:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// Stop closes the server's listener, effectively stopping the server from accepting new connections.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

// readHandler serves file read requests by opening and reading from the specified file in the server's directory.
func (s *Server) readHandler(filename string, rf io.ReaderFrom) error {
	file, err := os.Open(s.serveDir + "/" + filename)
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

	file, err := os.Create(s.serveDir + "/" + filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = wt.WriteTo(file)
	return err
}
