package handlers

import (
	"log/slog"

	"ignite/config"
)

// DatabaseStore interface for handlers to access database
type DatabaseStore interface {
	GetKV(bucket string, key []byte) ([]byte, error)
	PutKV(bucket string, key, value []byte) error
	DeleteKV(bucket string, key []byte) error
	GetAllKV(bucket string) (map[string][]byte, error)
	Close() error
}

// Handlers holds dependencies needed by HTTP handlers
type Handlers struct {
	DB     DatabaseStore
	Config *config.Config
	Logger *slog.Logger
}

// NewHandlers creates a new Handlers instance with dependencies
func NewHandlers(db DatabaseStore, cfg *config.Config, logger *slog.Logger) *Handlers {
	return &Handlers{
		DB:     db,
		Config: cfg,
		Logger: logger,
	}
}

// GetTFTPDir returns the TFTP directory from config
func (h *Handlers) GetTFTPDir() string {
	return h.Config.TFTP.Dir
}

// GetHTTPDir returns the HTTP directory from config
func (h *Handlers) GetHTTPDir() string {
	return h.Config.HTTP.Dir
}

// GetProvisionDir returns the provision directory from config
func (h *Handlers) GetProvisionDir() string {
	return h.Config.Provision.Dir
}

// GetDBBucket returns the database bucket name from config
func (h *Handlers) GetDBBucket() string {
	return h.Config.DB.Bucket
}
