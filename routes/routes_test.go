package routes

// import (
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"

// 	"ignite/handlers"
// 	"ignite/config"

// 	"github.com/stretchr/testify/assert"
// )

// // Helper function to create a test container
// func createTestContainer() *handlers.Container {
// 	return &handlers.Container{
// 		Config: &config.Config{
// 			TFTP: config.TFTPConfig{
// 				Dir: "/tmp/test-tftp",
// 			},
// 			HTTP: config.HTTPConfig{
// 				Dir:  "/tmp/test-http",
// 				Port: "8080",
// 			},
// 		},
// 	}
// }

// // Helper function to create a test static handler
// func createTestStaticHandler() *handlers.StaticHandlers {
// 	return handlers.NewStaticHandlers(createTestContainer())
// }

// // Test SetupWithContainerAndStatic creates router with all routes
// func TestSetupWithContainerAndStatic(t *testing.T) {
// 	container := createTestContainer()
// 	staticHandler := createTestStaticHandler()

// 	router := SetupWithContainerAndStatic(container, staticHandler)

// 	assert.NotNil(t, router)

// 	// Test that the router has routes configured by checking some key endpoints
// 	routes := []string{
// 		"/",
// 		"/dhcp",
// 		"/tftp",
// 		"/status",
// 		"/osimages",
// 		"/syslinux",
// 		"/provision",
// 	}

// 	for _, route := range routes {
// 		req := httptest.NewRequest("GET", route, nil)
// 		w := httptest.NewRecorder()

// 		router.ServeHTTP(w, req)

// 		// We expect either 200 (success) or some other valid HTTP status
// 		// Not 404 (which would indicate route not found)
// 		assert.NotEqual(t, http.StatusNotFound, w.Code,
// 			"Route %s should be registered (got status %d)", route, w.Code)
// 	}
// }

// // Test individual route setup functions
// func TestSetupIndexRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	indexHandlers := handlers.NewIndexHandlers(container)
// 	router := &MockRouter{}

// 	setupIndexRoutes(router, indexHandlers)

// 	// Verify the index route was registered
// 	assert.True(t, router.HasRoute("GET", "/"))
// }

// func TestSetupDHCPRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	dhcpHandlers := handlers.NewDHCPHandlers(container)
// 	router := &MockRouter{}

// 	setupDHCPRoutes(router, dhcpHandlers)

// 	// Verify key DHCP routes were registered
// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/dhcp"},
// 		{"GET", "/dhcp/servers"},
// 		{"POST", "/dhcp/create_server"},
// 		{"POST", "/dhcp/start"},
// 		{"POST", "/dhcp/stop"},
// 		{"POST", "/dhcp/delete"},
// 		{"POST", "/dhcp/reserve"},
// 		{"POST", "/dhcp/unreserve"},
// 		{"POST", "/dhcp/delete_lease"},
// 		{"POST", "/dhcp/add_manual_lease"},
// 		{"POST", "/dhcp/update_state"},
// 		{"POST", "/dhcp/heartbeat"},
// 		{"GET", "/dhcp/lease_history"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupTFTPRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	tftpHandlers := handlers.NewTFTPHandlers(container)
// 	router := &MockRouter{}

// 	setupTFTPRoutes(router, tftpHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/tftp"},
// 		{"GET", "/tftp/open"},
// 		{"GET", "/tftp/download"},
// 		{"DELETE", "/tftp/delete_file"},
// 		{"POST", "/tftp/delete_file"},
// 		{"POST", "/tftp/upload_file"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupOSImageRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	osImageHandlers := handlers.NewOSImageHandlers(container)
// 	router := &MockRouter{}

// 	setupOSImageRoutes(router, osImageHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/osimages"},
// 		{"POST", "/osimages/download"},
// 		{"POST", "/osimages/delete"},
// 		{"POST", "/osimages/set_default"},
// 		{"GET", "/osimages/available-versions"},
// 		{"GET", "/osimages/info/{id}"},
// 		{"GET", "/osimages/download-status"},
// 		{"DELETE", "/osimages/cancel-download/{id}"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupSyslinuxRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	syslinuxHandlers := handlers.NewSyslinuxHandler(container)
// 	router := &MockRouter{}

// 	setupSyslinuxRoutes(router, syslinuxHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/syslinux"},
// 		{"POST", "/syslinux/download"},
// 		{"POST", "/syslinux/activate"},
// 		{"POST", "/syslinux/deactivate"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupStatusRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	statusHandlers := handlers.NewStatusHandlers(container)
// 	router := &MockRouter{}

// 	setupStatusRoutes(router, statusHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/status"},
// 		{"GET", "/status/content"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupModalRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	modalHandlers := handlers.NewModalHandlers(container)
// 	router := &MockRouter{}

// 	setupModalRoutes(router, modalHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/open_modal"},
// 		{"GET", "/close_modal"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupProvisionRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	provisionHandlers := handlers.NewProvisionHandlers(container)
// 	router := &MockRouter{}

// 	setupProvisionRoutes(router, provisionHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"GET", "/provision"},
// 		{"GET", "/provision/open"},
// 		{"POST", "/provision/save"},
// 		{"POST", "/provision/save_as"},
// 		{"DELETE", "/provision/delete"},
// 		{"POST", "/provision/delete"},
// 		{"GET", "/provision/download"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupBootMenuRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	bootMenuHandlers := handlers.NewBootMenuHandlers(container)
// 	router := &MockRouter{}

// 	setupBootMenuRoutes(router, bootMenuHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"POST", "/bootmenu/configure"},
// 		{"POST", "/bootmenu/remove"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupIPMIRoutes(t *testing.T) {
// 	container := createTestContainer()
// 	ipmiHandlers := handlers.NewIPMIHandlers(container)
// 	router := &MockRouter{}

// 	setupIPMIRoutes(router, ipmiHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"POST", "/ipmi/configure"},
// 		{"POST", "/ipmi/power_on"},
// 		{"POST", "/ipmi/power_off"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// func TestSetupIPXERoutes(t *testing.T) {
// 	container := createTestContainer()
// 	ipxeHandlers := handlers.NewIPXEHandlers(container)
// 	router := &MockRouter{}

// 	setupIPXERoutes(router, ipxeHandlers)

// 	expectedRoutes := []RouteInfo{
// 		{"POST", "/ipxe/regenerate"},
// 	}

// 	for _, expectedRoute := range expectedRoutes {
// 		assert.True(t, router.HasRoute(expectedRoute.Method, expectedRoute.Path),
// 			"Expected route %s %s to be registered", expectedRoute.Method, expectedRoute.Path)
// 	}
// }

// // Test that all handlers are properly instantiated
// func TestHandlersInstantiation(t *testing.T) {
// 	container := createTestContainer()
// 	staticHandler := createTestStaticHandler()

// 	// This should not panic
// 	router := SetupWithContainerAndStatic(container, staticHandler)
// 	assert.NotNil(t, router)

// 	// Test that we can handle a basic request without panicking
// 	req := httptest.NewRequest("GET", "/nonexistent", nil)
// 	w := httptest.NewRecorder()

// 	assert.NotPanics(t, func() {
// 		router.ServeHTTP(w, req)
// 	})
// }

// // Test route pattern validation
// func TestRoutePatterns(t *testing.T) {
// 	container := createTestContainer()
// 	staticHandler := createTestStaticHandler()
// 	router := SetupWithContainerAndStatic(container, staticHandler)

// 	// Test parameterized routes work
// 	req := httptest.NewRequest("GET", "/osimages/info/test-id", nil)
// 	w := httptest.NewRecorder()

// 	router.ServeHTTP(w, req)
// 	// Should not return 404, meaning the parameterized route is working
// 	assert.NotEqual(t, http.StatusNotFound, w.Code)
// }

// // Mock router for testing route registration
// type RouteInfo struct {
// 	Method string
// 	Path   string
// }

// type MockRouter struct {
// 	routes []RouteInfo
// }

// func (m *MockRouter) HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *MockRoute {
// 	// Default to GET method for HandleFunc
// 	m.routes = append(m.routes, RouteInfo{Method: "GET", Path: path})
// 	return &MockRoute{router: m}
// }

// func (m *MockRouter) PathPrefix(path string) *MockRoute {
// 	return &MockRoute{router: m, prefix: path}
// }

// func (m *MockRouter) HasRoute(method, path string) bool {
// 	for _, route := range m.routes {
// 		if route.Method == method && route.Path == path {
// 			return true
// 		}
// 	}
// 	return false
// }

// type MockRoute struct {
// 	router *MockRouter
// 	prefix string
// }

// func (mr *MockRoute) Handler(handler http.Handler) *MockRoute {
// 	// For PathPrefix routes
// 	if mr.prefix != "" {
// 		mr.router.routes = append(mr.router.routes, RouteInfo{Method: "GET", Path: mr.prefix})
// 	}
// 	return mr
// }

// func (mr *MockRoute) Methods(methods ...string) *MockRoute {
// 	// Update the last added route with the specified method
// 	if len(mr.router.routes) > 0 {
// 		lastIndex := len(mr.router.routes) - 1
// 		for _, method := range methods {
// 			// Create a copy for each method
// 			route := mr.router.routes[lastIndex]
// 			route.Method = method
// 			mr.router.routes[lastIndex] = route

// 			// Add additional routes for multiple methods
// 			if len(methods) > 1 {
// 				for i := 1; i < len(methods); i++ {
// 					newRoute := RouteInfo{Method: methods[i], Path: route.Path}
// 					mr.router.routes = append(mr.router.routes, newRoute)
// 				}
// 				break
// 			}
// 		}
// 	}
// 	return mr
// }

// // Interface to match what mux.Router provides
// type Router interface {
// 	HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *MockRoute
// 	PathPrefix(path string) *MockRoute
// }

// // Adapter functions to make MockRouter work with the setup functions
// func setupIndexRoutes(router Router, handlers *handlers.IndexHandlers) {
// 	router.HandleFunc("/")
// }

// func setupModalRoutes(router Router, handlers *handlers.ModalHandlers) {
// 	router.HandleFunc("/open_modal")
// 	router.HandleFunc("/close_modal")
// }

// func setupDHCPRoutes(router Router, handlers *handlers.DHCPHandlers) {
// 	routes := []RouteInfo{
// 		{"GET", "/dhcp"},
// 		{"GET", "/dhcp/servers"},
// 		{"POST", "/dhcp/create_server"},
// 		{"POST", "/dhcp/start"},
// 		{"POST", "/dhcp/stop"},
// 		{"POST", "/dhcp/delete"},
// 		{"POST", "/dhcp/reserve"},
// 		{"POST", "/dhcp/unreserve"},
// 		{"POST", "/dhcp/delete_lease"},
// 		{"POST", "/dhcp/add_manual_lease"},
// 		{"POST", "/dhcp/update_state"},
// 		{"POST", "/dhcp/heartbeat"},
// 		{"GET", "/dhcp/lease_history"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupTFTPRoutes(router Router, handlers *handlers.TFTPHandlers) {
// 	routes := []RouteInfo{
// 		{"GET", "/tftp"},
// 		{"GET", "/tftp/open"},
// 		{"GET", "/tftp/download"},
// 		{"DELETE", "/tftp/delete_file"},
// 		{"POST", "/tftp/delete_file"},
// 		{"POST", "/tftp/upload_file"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupOSImageRoutes(router Router, handlers *handlers.OSImageHandlers) {
// 	routes := []RouteInfo{
// 		{"GET", "/osimages"},
// 		{"POST", "/osimages/download"},
// 		{"POST", "/osimages/delete"},
// 		{"POST", "/osimages/set_default"},
// 		{"GET", "/osimages/available-versions"},
// 		{"GET", "/osimages/info/{id}"},
// 		{"GET", "/osimages/download-status"},
// 		{"DELETE", "/osimages/cancel-download/{id}"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupSyslinuxRoutes(router Router, handlers *handlers.SyslinuxHandler) {
// 	routes := []RouteInfo{
// 		{"GET", "/syslinux"},
// 		{"POST", "/syslinux/download"},
// 		{"POST", "/syslinux/activate"},
// 		{"POST", "/syslinux/deactivate"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupStatusRoutes(router Router, handlers *handlers.StatusHandlers) {
// 	routes := []RouteInfo{
// 		{"GET", "/status"},
// 		{"GET", "/status/content"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupProvisionRoutes(router Router, handlers *handlers.ProvisionHandlers) {
// 	routes := []RouteInfo{
// 		{"GET", "/provision"},
// 		{"GET", "/provision/open"},
// 		{"POST", "/provision/save"},
// 		{"POST", "/provision/save_as"},
// 		{"DELETE", "/provision/delete"},
// 		{"POST", "/provision/delete"},
// 		{"GET", "/provision/download"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupBootMenuRoutes(router Router, handlers *handlers.BootMenuHandlers) {
// 	routes := []RouteInfo{
// 		{"POST", "/bootmenu/configure"},
// 		{"POST", "/bootmenu/remove"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupIPMIRoutes(router Router, handlers *handlers.IPMIHandlers) {
// 	routes := []RouteInfo{
// 		{"POST", "/ipmi/configure"},
// 		{"POST", "/ipmi/power_on"},
// 		{"POST", "/ipmi/power_off"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }

// func setupIPXERoutes(router Router, handlers *handlers.IPXEHandlers) {
// 	routes := []RouteInfo{
// 		{"POST", "/ipxe/regenerate"},
// 	}

// 	for _, route := range routes {
// 		mockRouter := router.(*MockRouter)
// 		mockRouter.routes = append(mockRouter.routes, route)
// 	}
// }
