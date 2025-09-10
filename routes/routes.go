package routes

import (
	"ignite/handlers"

	"github.com/gorilla/mux"
)

// SetupWithContainerAndStatic configures routes with dependency injection and static files
func SetupWithContainerAndStatic(container *handlers.Container, staticHandler *handlers.StaticHandlers) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	// Setup static file serving
	setupStaticRoutes(router, staticHandler)

	// Create handler instances with dependencies
	dhcpHandlers := handlers.NewDHCPHandlers(container)

	// Setup API routes
	setupDHCPRoutes(router, dhcpHandlers)

	return router
}

func setupStaticRoutes(router *mux.Router, staticHandler *handlers.StaticHandlers) {
	// Serve static files from /public/http/ path
	router.PathPrefix("/public/http/").HandlerFunc(staticHandler.ServeStatic).Methods("GET")
}

// setupDHCPRoutes configures DHCP-related routes
func setupDHCPRoutes(router *mux.Router, handlers *handlers.DHCPHandlers) {
	// GET routes
	router.HandleFunc("/dhcp", handlers.GetDHCPServers).Methods("GET").Name("DHCPPage")
	router.HandleFunc("/dhcp/servers", handlers.GetDHCPServers).Methods("GET").Name("DHCPServers")

	// POST routes
	router.HandleFunc("/dhcp/start", handlers.StartDHCPServer).Methods("POST").Name("StartDHCP")
	router.HandleFunc("/dhcp/stop", handlers.StopDHCPServer).Methods("POST").Name("StopDHCP")
	router.HandleFunc("/dhcp/delete", handlers.DeleteDHCPServer).Methods("POST").Name("DeleteDHCP")
	router.HandleFunc("/dhcp/submit_dhcp", handlers.SubmitDHCPServer).Methods("POST").Name("SubmitDHCP")
	router.HandleFunc("/dhcp/submit_reserve", handlers.ReserveLease).Methods("POST").Name("ReserveLease")
	router.HandleFunc("/dhcp/remove_reserve", handlers.UnreserveLease).Methods("POST").Name("UnreserveLease")
	router.HandleFunc("/dhcp/delete_lease", handlers.DeleteLease).Methods("POST").Name("DeleteLease")
}
