package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
	"ignite/routes"
	"ignite/tftp"

	"github.com/gorilla/mux"
)

//go:embed public/http/*
var staticFS embed.FS

func main() {
	if err := setupAndRun(); err != nil {
		log.Fatal(err)
	}
}

// setupAndRun initializes the database, DHCP servers, TFTP server, and HTTP server, then starts the HTTP server.
//
// Returns:
//   - error: If there's an error during setup or server start.
func setupAndRun() error {
	db, err := setupDatabase()
	if err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer db.Close()

	setupDHCPServers(db)
	setupTFTPServer()
	router := setupHTTPServer()

	log.Printf("BoltDB server initialized...")
	log.Printf("TFTP server serving directory: %s", config.Defaults.TFTP.Dir)
	log.Printf("HTTP server starting on port: %s directory: %s", config.Defaults.HTTP.Port, config.Defaults.HTTP.Dir)

	return http.ListenAndServe(":"+config.Defaults.HTTP.Port, router)
}

// setupDatabase initializes and returns a BoltDB database instance.
//
// Returns:
//   - *db.BoltKV: Pointer to the initialized BoltDB instance.
//   - error: If initialization fails.
func setupDatabase() (*db.BoltKV, error) {
	boltdb, err := db.Init()
	if err != nil {
		return nil, fmt.Errorf("error opening BoltDB: %v", err)
	}
	return boltdb, nil
}

// setupDHCPServers retrieves DHCP server configurations from the database and initializes them.
//
// Parameters:
//   - db: Pointer to the BoltDB database instance.
func setupDHCPServers(db *db.BoltKV) {
	bucket := config.Defaults.DB.Bucket
	kv, err := db.GetAllKV(bucket)
	if err != nil {
		log.Printf("Error getting KV: %v", err)
		return
	}

	fmt.Print("Getting DHCP Servers...")
	if len(kv) == 0 {
		fmt.Println("No DHCP servers found.")
	} else {
		handler := dhcp.DHCPHandler{}
		for k, v := range kv {
			fmt.Printf(" %s\n", k)
			if err := json.Unmarshal(v, &handler); err != nil {
				log.Printf("Error unmarshaling DHCP server %s: %v", k, err)
				continue
			}
			handler.Started = false
			handler.UpdateDBState()
		}
	}
}

// setupTFTPServer initializes and starts the TFTP server.
//
// Returns:
//   - *tftp.Server: Pointer to the started TFTP server instance.
func setupTFTPServer() *tftp.Server {
	tftpDir := config.Defaults.TFTP.Dir
	tftpServer := tftp.NewServer(tftpDir)
	if err := tftpServer.Start(); err != nil {
		log.Fatalf("Error starting TFTP server: %v", err)
	}
	return tftpServer
}

// setupHTTPServer configures and returns an HTTP router with necessary routes for serving static files and provisioning.
//
// Returns:
//   - *mux.Router: Configured router for HTTP server.
func setupHTTPServer() *mux.Router {
	router := routes.Setup()

	publicFS, err := fs.Sub(staticFS, "public/http")
	if err != nil {
		log.Panic(err)
	}
	router.PathPrefix("/public/http/").Handler(http.StripPrefix("/public/http/", http.FileServer(http.FS(publicFS))))

	provDir := config.Defaults.Provision.Dir
	configsPath := filepath.Join(provDir, "configs")
	templatesPath := filepath.Join(provDir, "templates")

	router.PathPrefix("/public/provision/configs/").Handler(http.StripPrefix("/public/provision/configs/", http.FileServer(http.Dir(configsPath))))
	router.PathPrefix("/public/provision/templates/").Handler(http.StripPrefix("/public/provision/templates/", http.FileServer(http.Dir(templatesPath))))

	return router
}
