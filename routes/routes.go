package routes

import (
	"ignite/handlers"
	"ignite/osimage"
	"net/http"

	"github.com/gorilla/mux"
)

// guardedRouter serves OS image requests before gorilla/mux rewrites them.
// mux cleans ".." into a redirect and answers a method mismatch without
// an Allow header. This handler sees the original path and method.
type guardedRouter struct {
	*mux.Router
	boot http.Handler
}

func (g *guardedRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.boot != nil && handlers.BootFileRequest(r) {
		g.boot.ServeHTTP(w, r)
		return
	}
	g.Router.ServeHTTP(w, r)
}

// SetupWithContainerAndStatic configures routes with dependency injection and static files
func SetupWithContainerAndStatic(container *handlers.Container, staticHandler *handlers.StaticHandlers) *guardedRouter {
	router := mux.NewRouter().StrictSlash(true)

	// Setup static file serving
	setupStaticRoutes(router, staticHandler)
	boot := setupBootFileRoute(router, container)

	// Create handler instances with dependencies
	authHandlers := handlers.NewAuthHandlers(container)
	dhcpHandlers := handlers.NewDHCPHandlers(container)
	tftpHandlers := handlers.NewTFTPHandlers(container)
	provisionHandlers := handlers.NewProvisionHandlers(container)
	bootMenuHandlers := handlers.NewBootMenuHandlers(container)
	ipmiHandlers := handlers.NewIPMIHandlers(container)
	statusHandlers := handlers.NewStatusHandlers(container)
	dashboardHandlers := handlers.NewDashboardHandlers(container)
	preferenceHandlers := handlers.NewPreferenceHandlers(container.Prefs)
	modalHandlers := handlers.NewModalHandlers(container)
	indexHandlers := handlers.NewIndexHandlers(container)
	osImageHandlers := handlers.NewOSImageHandlers(container)
	syslinuxHandlers := handlers.NewSyslinuxHandler(container)
	ipxeHandlers := handlers.NewIPXEHandlers(container)

	// Apply authentication middleware to the entire router
	router.Use(handlers.AuthMiddleware)

	// Setup all routes (middleware will handle auth checks)
	setupAuthRoutes(router, authHandlers)
	setupIndexRoutes(router, indexHandlers)
	setupModalRoutes(router, modalHandlers)
	setupDHCPRoutes(router, dhcpHandlers)
	setupTFTPRoutes(router, tftpHandlers)
	setupProvisionRoutes(router, provisionHandlers)
	setupBootMenuRoutes(router, bootMenuHandlers)
	setupIPMIRoutes(router, ipmiHandlers)
	setupOSImageRoutes(router, osImageHandlers)
	setupSyslinuxRoutes(router, syslinuxHandlers)
	setupIPXERoutes(router, ipxeHandlers)
	setupStatusRoutes(router, statusHandlers)
	setupDashboardRoutes(router, dashboardHandlers)
	setupPreferenceRoutes(router, preferenceHandlers)

	router.NotFoundHandler = http.HandlerFunc(handlers.NotFound)
	router.MethodNotAllowedHandler = http.HandlerFunc(handlers.MethodNotAllowed)

	return &guardedRouter{Router: router, boot: boot}
}

// setupBootFileRoute serves downloaded kernels and initrds at the same
// relative URL the iPXE menu emits. PXE clients have no session.
// Every method is registered. The handler returns 405 for the ones it
// does not serve, with Allow: GET, HEAD.
func setupBootFileRoute(router *mux.Router, container *handlers.Container) http.Handler {
	if container == nil || container.Config == nil || container.Config.TFTP.Dir == "" {
		return nil
	}
	handler := handlers.NewBootFileHandler(container.Config.TFTP.Dir)
	prefix := "/" + osimage.DirName
	router.Handle(prefix, handler)
	router.PathPrefix(prefix + "/").Handler(handler)
	return handler
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
	router.HandleFunc("/dhcp/add_manual_lease", handlers.AddManualLease).Methods("POST").Name("AddManualLease")

	// State management API routes. The admin UI uses /dhcp/lease/state and
	// stays behind the session cookie. Host callbacks are the two routes
	// below; AuthMiddleware lets them through and the handlers require the
	// per-lease provision token.
	router.HandleFunc("/dhcp/lease/state", handlers.UpdateLeaseState).Methods("POST").Name("UpdateLeaseState")
	router.HandleFunc("/dhcp/lease/history", handlers.GetLeaseStateHistory).Methods("GET").Name("GetLeaseStateHistory")
	router.HandleFunc("/dhcp/lease/{mac}/heartbeat", handlers.RecordHeartbeat).Methods("POST").Name("RecordHeartbeat")
	router.HandleFunc("/dhcp/lease/{mac}/state", handlers.HostUpdateLeaseState).Methods("POST").Name("HostUpdateLeaseState")
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
	router.HandleFunc("/provision/delete-file", handlers.DeleteFile).Methods("POST").Name("DeleteFile")
}

// setupBootMenuRoutes configures PXE boot menu routes
func setupBootMenuRoutes(router *mux.Router, handlers *handlers.BootMenuHandlers) {
	// POST routes
	router.HandleFunc("/pxe/submit_menu", handlers.SubmitBootMenu).Methods("POST").Name("SubmitBootMenu")
	// Installer download. AuthMiddleware lets it through; the handler allows
	// only the lease IP or the provision token.
	router.HandleFunc("/configs/{configType}/{name}", handlers.ServeRenderedConfig).Methods("GET").Name("RenderedConfig")
}

// setupIPMIRoutes configures IPMI management routes
func setupIPMIRoutes(router *mux.Router, handlers *handlers.IPMIHandlers) {
	// POST routes
	router.HandleFunc("/pxe/submit_ipmi", handlers.SubmitIPMI).Methods("POST").Name("SubmitIPMI")
}

// setupDashboardRoutes configures the Overview summary fragment.
func setupDashboardRoutes(router *mux.Router, handlers *handlers.DashboardHandlers) {
	router.HandleFunc("/dashboard/summary", handlers.Summary).Methods("GET").Name("DashboardSummary")
}

// setupPreferenceRoutes configures the per-user UI mode endpoints.
func setupPreferenceRoutes(router *mux.Router, handlers *handlers.PreferenceHandlers) {
	router.HandleFunc("/api/preferences/mode", handlers.GetMode).Methods("GET").Name("GetUIMode")
	router.HandleFunc("/api/preferences/mode", handlers.SetMode).Methods("POST").Name("SetUIMode")
}

// setupStatusRoutes configures status routes.
// GET /status redirects to Overview. GET /status/content is the fragment
// Overview polls.
func setupStatusRoutes(router *mux.Router, handlers *handlers.StatusHandlers) {
	router.HandleFunc("/status", handlers.HandleStatusPage).Methods("GET").Name("Status")
	router.HandleFunc("/status/content", handlers.HandleStatusContent).Methods("GET").Name("StatusContent")
}

// setupOSImageRoutes configures OS image management routes
func setupOSImageRoutes(router *mux.Router, handlers *handlers.OSImageHandlers) {
	// GET routes
	router.HandleFunc("/osimages", handlers.OSImagesPage).Methods("GET").Name("OSImagesPage")
	router.HandleFunc("/osimages/download/status/{id}", handlers.GetDownloadStatus).Methods("GET").Name("GetDownloadStatus")
	router.HandleFunc("/osimages/info/{id}", handlers.GetOSImageInfo).Methods("GET").Name("GetOSImageInfo")
	router.HandleFunc("/osimages/available-versions", handlers.GetAvailableVersions).Methods("GET").Name("GetAvailableVersions")
	router.HandleFunc("/osimages/catalog", handlers.GetCatalog).Methods("GET").Name("GetOSImageCatalog")

	// POST routes
	router.HandleFunc("/osimages/catalog/refresh", handlers.RefreshCatalog).Methods("POST").Name("RefreshOSImageCatalog")
	router.HandleFunc("/osimages/download", handlers.DownloadOSImage).Methods("POST").Name("DownloadOSImage")
	router.HandleFunc("/osimages/set-default/{id}", handlers.SetDefaultVersion).Methods("POST").Name("SetDefaultVersion")
	router.HandleFunc("/osimages/cancel/{id}", handlers.CancelDownload).Methods("POST").Name("CancelDownload")

	// DELETE routes
	router.HandleFunc("/osimages/delete/{id}", handlers.DeleteOSImage).Methods("DELETE").Name("DeleteOSImage")
}

// setupSyslinuxRoutes configures Syslinux management routes
func setupSyslinuxRoutes(router *mux.Router, handlers *handlers.SyslinuxHandler) {
	handlers.RegisterRoutes(router)
}

// setupIPXERoutes configures iPXE configuration routes
func setupIPXERoutes(router *mux.Router, handlers *handlers.IPXEHandlers) {
	// GET routes - for viewing/generating config
	router.HandleFunc("/ipxe/config", handlers.GenerateConfig).Methods("GET").Name("GetIPXEConfig")

	// POST routes - for updating config file
	router.HandleFunc("/ipxe/update", handlers.UpdateConfigFile).Methods("POST").Name("UpdateIPXEConfig")
}

// setupAuthRoutes configures authentication routes
func setupAuthRoutes(router *mux.Router, handlers *handlers.AuthHandlers) {
	// GET routes
	router.HandleFunc("/login", handlers.LoginPage).Methods("GET").Name("LoginPage")

	// POST routes
	router.HandleFunc("/auth/login", handlers.Login).Methods("POST").Name("Login")
	router.HandleFunc("/auth/logout", handlers.Logout).Methods("POST").Name("Logout")
	router.HandleFunc("/auth/change-password", handlers.ChangePassword).Methods("POST").Name("ChangePassword")
}
