package routes

import (
	"ignite/handlers"
	"net/http"

	"github.com/gorilla/mux"
)

// SetupWithContainerAndStatic configures routes with dependency injection and static files
func SetupWithContainerAndStatic(container *handlers.Container, staticHandler *handlers.StaticHandlers) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	// Setup static file serving
	setupStaticRoutes(router, staticHandler)

	// Create handler instances with dependencies
	dhcpHandlers := handlers.NewDHCPHandlers(container)
	tftpHandlers := handlers.NewTFTPHandlers(container)
	provisionHandlers := handlers.NewProvisionHandlers(container)
	bootMenuHandlers := handlers.NewBootMenuHandlers(container)
	ipmiHandlers := handlers.NewIPMIHandlers(container)
	statusHandlers := handlers.NewStatusHandlers(container)
	modalHandlers := handlers.NewModalHandlers(container)
	indexHandlers := handlers.NewIndexHandlers(container)
	osImageHandlers := handlers.NewOSImageHandlers(container)

	// Setup all routes
	setupIndexRoutes(router, indexHandlers)
	setupModalRoutes(router, modalHandlers)
	setupDHCPRoutes(router, dhcpHandlers)
	setupTFTPRoutes(router, tftpHandlers)
	setupProvisionRoutes(router, provisionHandlers)
	setupBootMenuRoutes(router, bootMenuHandlers)
	setupIPMIRoutes(router, ipmiHandlers)
	setupOSImageRoutes(router, osImageHandlers)
	setupStatusRoutes(router, statusHandlers)

	return router
}

func setupStaticRoutes(router *mux.Router, staticHandler *handlers.StaticHandlers) {
	// Serve static files from /public/http/ path using built-in FileServer
	// Use Go's built-in FileServer with the embedded filesystem
	fileServer := http.FileServer(http.FS(staticHandler.GetFS()))
	// Strip the /public/http prefix and serve from the embedded filesystem root
	router.PathPrefix("/public/http/").Handler(http.StripPrefix("/", fileServer)).Methods("GET")
}

// setupIndexRoutes configures the main index page route
func setupIndexRoutes(router *mux.Router, handlers *handlers.IndexHandlers) {
	router.HandleFunc("/", handlers.Index).Methods("GET").Name("Index")
}

// setupModalRoutes configures modal-related routes
func setupModalRoutes(router *mux.Router, handlers *handlers.ModalHandlers) {
	router.HandleFunc("/open_modal", handlers.OpenModalHandler).Methods("GET").Name("OpenModal")
	router.HandleFunc("/close_modal", handlers.CloseModalHandler).Methods("GET").Name("CloseModal")
}

// setupDHCPRoutes configures DHCP-related routes
func setupDHCPRoutes(router *mux.Router, handlers *handlers.DHCPHandlers) {
	// GET routes
	router.HandleFunc("/dhcp", handlers.HandleDHCPPage).Methods("GET").Name("DHCPPage")
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

// setupTFTPRoutes configures TFTP file management routes
func setupTFTPRoutes(router *mux.Router, handlers *handlers.TFTPHandlers) {
	// GET routes
	router.HandleFunc("/tftp", handlers.HandleTFTPPage).Methods("GET").Name("TFTPPage")
	router.HandleFunc("/tftp/open", handlers.HandleTFTPPage).Methods("GET").Name("OpenTFTP")
	router.HandleFunc("/tftp/download", handlers.HandleDownload).Methods("GET").Name("DownloadFile")
	router.HandleFunc("/tftp/view", handlers.ViewFile).Methods("GET").Name("ViewFile")
	router.HandleFunc("/tftp/serve", handlers.ServeFile).Methods("GET").Name("ServeFile")

	// POST routes
	router.HandleFunc("/tftp/delete_file", handlers.HandleDelete).Methods("POST").Name("DeleteFile")
	router.HandleFunc("/tftp/upload_file", handlers.HandleUpload).Methods("POST").Name("UploadFile")
}

// setupProvisionRoutes configures provisioning template management routes
func setupProvisionRoutes(router *mux.Router, handlers *handlers.ProvisionHandlers) {
	// GET routes
	router.HandleFunc("/provision", handlers.HomeHandler).Methods("GET").Name("Provision")
	router.HandleFunc("/prov/gettemplates", handlers.HandleFileOptions).Methods("GET").Name("GetTemplateOptions")
	router.HandleFunc("/prov/loadtemplate", handlers.LoadTemplate).Methods("GET").Name("LoadTemplate")
	router.HandleFunc("/prov/getconfigs", handlers.HandleConfigOptions).Methods("GET").Name("GetConfigOptions")
	router.HandleFunc("/prov/loadconfig", handlers.LoadConfig).Methods("GET").Name("LoadConfig")
	router.HandleFunc("/prov/getfilename", handlers.UpdateFilename).Methods("GET").Name("GetFilename")

	// New API endpoints for modern interface
	router.HandleFunc("/provision/load-file", handlers.LoadFileContent).Methods("GET").Name("LoadFileContent")
	router.HandleFunc("/provision/gallery", handlers.GetTemplateGallery).Methods("GET").Name("TemplateGallery")

	// POST routes
	router.HandleFunc("/prov/newtemplate", handlers.HandleNewTemplate).Methods("POST").Name("NewTemplate")
	router.HandleFunc("/prov/save", handlers.HandleSave).Methods("POST").Name("SaveFile")
	router.HandleFunc("/provision/save-file", handlers.SaveFileContent).Methods("POST").Name("SaveFileContent")
}

// setupBootMenuRoutes configures PXE boot menu routes
func setupBootMenuRoutes(router *mux.Router, handlers *handlers.BootMenuHandlers) {
	// POST routes
	router.HandleFunc("/pxe/submit_menu", handlers.SubmitBootMenu).Methods("POST").Name("SubmitBootMenu")
}

// setupIPMIRoutes configures IPMI management routes
func setupIPMIRoutes(router *mux.Router, handlers *handlers.IPMIHandlers) {
	// POST routes
	router.HandleFunc("/pxe/submit_ipmi", handlers.SubmitIPMI).Methods("POST").Name("SubmitIPMI")
}

// setupStatusRoutes configures status monitoring routes
func setupStatusRoutes(router *mux.Router, handlers *handlers.StatusHandlers) {
	// GET routes
	router.HandleFunc("/status", handlers.HandleStatusPage).Methods("GET").Name("Status")
	router.HandleFunc("/status/content", handlers.HandleStatusContent).Methods("GET").Name("StatusContent")
}

// setupOSImageRoutes configures OS image management routes
func setupOSImageRoutes(router *mux.Router, handlers *handlers.OSImageHandlers) {
	// GET routes
	router.HandleFunc("/osimages", handlers.OSImagesPage).Methods("GET").Name("OSImagesPage")
	router.HandleFunc("/osimages/list", handlers.ListOSImages).Methods("GET").Name("ListOSImages")
	router.HandleFunc("/osimages/download/status/{id}", handlers.GetDownloadStatus).Methods("GET").Name("GetDownloadStatus")
	router.HandleFunc("/osimages/info/{id}", handlers.GetOSImageInfo).Methods("GET").Name("GetOSImageInfo")
	router.HandleFunc("/osimages/versions", handlers.GetAvailableVersions).Methods("GET").Name("GetAvailableVersions")
	router.HandleFunc("/osimages/by-os", handlers.GetOSImagesByOS).Methods("GET").Name("GetOSImagesByOS")
	
	// POST routes
	router.HandleFunc("/osimages/download", handlers.DownloadOSImage).Methods("POST").Name("DownloadOSImage")
	router.HandleFunc("/osimages/set-default/{id}", handlers.SetDefaultVersion).Methods("POST").Name("SetDefaultVersion")
	router.HandleFunc("/osimages/cancel/{id}", handlers.CancelDownload).Methods("POST").Name("CancelDownload")
	
	// DELETE routes
	router.HandleFunc("/osimages/delete/{id}", handlers.DeleteOSImage).Methods("DELETE").Name("DeleteOSImage")
}
