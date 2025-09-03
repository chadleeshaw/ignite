package routes

import (
	"github.com/gorilla/mux"

	h "ignite/handlers"
	ih "ignite/internal/handlers"
)

// Setup configures and returns a new router with all defined routes for the application.
func Setup(handlers *ih.Handlers) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	// GET routes for serving pages and retrieving data.
	setupGetRoutes(router, handlers)

	// POST routes for handling form submissions and server control actions.
	setupPostRoutes(router, handlers)

	return router
}

// setupGetRoutes defines all routes that handle GET requests.
func setupGetRoutes(router *mux.Router, handlers *ih.Handlers) {
	router.HandleFunc("/", h.Index).Methods("GET").Name("Index")
	router.HandleFunc("/open_modal", handlers.OpenModalHandler).Methods("GET").Name("OpenModal")
	router.HandleFunc("/close_modal", h.CloseModalHandler).Methods("GET").Name("CloseModal")
	router.HandleFunc("/dhcp", handlers.HandleDHCPPage).Methods("GET").Name("DHCPPage")
	router.HandleFunc("/dhcp/servers", handlers.GetDHCPServers).Methods("GET").Name("DHCPServers")
	router.HandleFunc("/status", h.HandleStatusPage).Methods("GET").Name("Status")
	router.HandleFunc("/provision", h.HomeHandler).Methods("GET").Name("Provision")
	router.HandleFunc("/tftp", h.HandleTFTPPage).Methods("GET").Name("TFTPPage")
	router.HandleFunc("/tftp/open", h.HandleTFTPPage).Methods("GET").Name("OpenTFTP")
	router.HandleFunc("/tftp/download", h.HandleDownload).Methods("GET").Name("DownloadFile")
	router.HandleFunc("/tftp/view", h.ViewFile).Methods("GET").Name("ViewFile")
	router.HandleFunc("/tftp/serve", h.ServeFile).Methods("GET").Name("ServeFile")
	router.HandleFunc("/prov/gettemplates", h.HandleFileOptions).Methods("GET").Name("GetTemplateOptions")
	router.HandleFunc("/prov/loadtemplate", h.LoadTemplate).Methods("GET").Name("LoadTemplate")
	router.HandleFunc("/prov/getconfigs", h.HandleConfigOptions).Methods("GET").Name("GetConfigOptions")
	router.HandleFunc("/prov/loadconfig", h.LoadConfig).Methods("GET").Name("LoadConfig")
	router.HandleFunc("/prov/getfilename", h.UpdateFilename).Methods("GET").Name("GetFilename")
}

// setupPostRoutes defines all routes that handle POST requests.
func setupPostRoutes(router *mux.Router, handlers *ih.Handlers) {
	router.HandleFunc("/dhcp/start", handlers.StartDHCPServer).Methods("POST").Name("StartDHCP")
	router.HandleFunc("/dhcp/stop", handlers.StopDHCPServer).Methods("POST").Name("StopDHCP")
	router.HandleFunc("/dhcp/delete", handlers.DeleteDHCPServer).Methods("POST").Name("DeleteDHCP")
	router.HandleFunc("/dhcp/submit_dhcp", handlers.SubmitDHCPServer).Methods("POST").Name("SubmitDHCP")
	router.HandleFunc("/dhcp/submit_reserve", handlers.ReserveLease).Methods("POST").Name("ReserveLease")
	router.HandleFunc("/dhcp/remove_reserve", handlers.UnreserveLease).Methods("POST").Name("UnreserveLease")
	router.HandleFunc("/dhcp/delete_lease", handlers.DeleteLease).Methods("POST").Name("DeleteLease")
	router.HandleFunc("/tftp/delete_file", h.HandleDelete).Methods("POST").Name("DeleteFile")
	router.HandleFunc("/tftp/upload_file", h.HandleUpload).Methods("POST").Name("UploadFile")
	router.HandleFunc("/pxe/submit_menu", handlers.SubmitBootMenu).Methods("POST").Name("SubmitBootMenu")
	router.HandleFunc("/pxe/submit_ipmi", handlers.SubmitIPMI).Methods("POST").Name("SubmitIPMI")
	router.HandleFunc("/prov/newtemplate", h.HandleNewTemplate).Methods("POST").Name("NewTemplate")
	router.HandleFunc("/prov/save", h.HandleSave).Methods("POST").Name("SaveFile")
}
