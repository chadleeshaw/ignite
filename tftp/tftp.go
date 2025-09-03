package tftp

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"ignite/internal/errors"
	"ignite/internal/validation"

	v3 "github.com/pin/tftp/v3"
)

// Server manages the TFTP server, handling file read and write operations.
type Server struct {
	serveDir   string
	listener   *net.UDPConn
	tftpServer *v3.Server
	logger     *slog.Logger
}

// NewServer creates and returns a new TFTP server instance with the specified directory for serving files.
func NewServer(serveDir string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		serveDir: serveDir,
		logger:   logger,
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
			s.logger.Error("TFTP server stopped with error", slog.String("error", err.Error()))
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
	// Validate and secure the file path
	safePath, err := validation.ValidateFilePath(s.serveDir, filename)
	if err != nil {
		s.logger.Warn("TFTP read request blocked", 
			slog.String("filename", filename),
			slog.String("error", err.Error()),
		)
		return errors.NewTFTPError("validate_path", err)
	}

	s.logger.Info("TFTP read request", slog.String("filename", filename))

	file, err := os.Open(safePath)
	if err != nil {
		s.logger.Error("Failed to open file for TFTP read",
			slog.String("filename", filename),
			slog.String("path", safePath),
			slog.String("error", err.Error()),
		)
		return errors.NewFileSystemError("open_file", err)
	}
	defer file.Close()

	bytesRead, err := rf.ReadFrom(file)
	if err != nil {
		s.logger.Error("Failed to read file for TFTP",
			slog.String("filename", filename),
			slog.String("error", err.Error()),
		)
		return errors.NewTFTPError("read_file", err)
	}

	s.logger.Info("TFTP read completed", 
		slog.String("filename", filename),
		slog.Int64("bytes", bytesRead),
	)
	return nil
}

// writeHandler handles file write requests by creating a new file or overwriting an existing one in the server's directory.
func (s *Server) writeHandler(filename string, wt io.WriterTo) error {
	// Validate filename for safety
	if err := validation.ValidateFilename(filename); err != nil {
		s.logger.Warn("TFTP write request blocked - invalid filename", 
			slog.String("filename", filename),
			slog.String("error", err.Error()),
		)
		return errors.NewValidationError("validate_filename", err)
	}

	// Validate and secure the file path
	safePath, err := validation.ValidateFilePath(s.serveDir, filename)
	if err != nil {
		s.logger.Warn("TFTP write request blocked - invalid path", 
			slog.String("filename", filename),
			slog.String("error", err.Error()),
		)
		return errors.NewTFTPError("validate_path", err)
	}

	s.logger.Info("TFTP write request", slog.String("filename", filename))

	// Ensure the directory exists
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.logger.Error("Failed to create directory for TFTP write",
			slog.String("directory", dir),
			slog.String("error", err.Error()),
		)
		return errors.NewFileSystemError("create_directory", err)
	}

	file, err := os.Create(safePath)
	if err != nil {
		s.logger.Error("Failed to create file for TFTP write",
			slog.String("filename", filename),
			slog.String("path", safePath),
			slog.String("error", err.Error()),
		)
		return errors.NewFileSystemError("create_file", err)
	}
	defer file.Close()

	bytesWritten, err := wt.WriteTo(file)
	if err != nil {
		s.logger.Error("Failed to write file for TFTP",
			slog.String("filename", filename),
			slog.String("error", err.Error()),
		)
		return errors.NewTFTPError("write_file", err)
	}

	s.logger.Info("TFTP write completed", 
		slog.String("filename", filename),
		slog.Int64("bytes", bytesWritten),
	)
	return nil
}
