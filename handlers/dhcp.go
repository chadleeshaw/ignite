package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
)

// DHCPServer represents details of a DHCP server instance.
type DHCPServer struct {
	TFTPIP string      `json:"tftpip"`
	Status string      `json:"status"`
	Leases []DHCPLease `json:"leases"`
}

// DHCPLease represents a single lease by a DHCP server.
type DHCPLease struct {
	IP     string        `json:"ip"`
	MAC    string        `json:"mac"`
	Static bool          `json:"static"`
	Menu   dhcp.BootMenu `json:"menu,omitempty"`
	IPMI   dhcp.IPMI     `json:"ipmi,omitempty"`
}

// DHCPPageData holds all the data needed to render the DHCP management page.
type DHCPPageData struct {
	Title   string       `json:"title"`
	Servers []DHCPServer `json:"servers"`
}

// HandleDHCPPage renders the DHCP server management page using structured data.
func HandleDHCPPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()
	data, err := getDHCPData()
	if err != nil {
		http.Error(w, "Failed to retrieve DHCP server data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := templates["dhcp"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetDHCPServers returns JSON data of all DHCP servers using structured data.
func GetDHCPServers(w http.ResponseWriter, r *http.Request) {
	data, err := getDHCPData()
	if err != nil {
		http.Error(w, "Failed to retrieve DHCP server data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode server data to JSON", http.StatusInternalServerError)
	}
}

// getDHCPData retrieves and formats data about all DHCP servers for display.
func getDHCPData() (*DHCPPageData, error) {
	data := &DHCPPageData{
		Title:   "DHCP Leases",
		Servers: make([]DHCPServer, 0),
	}

	dhcpHandlers, err := dhcp.GetAllDHCPServers()
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP servers: %v", err)
	} else {
		for _, handler := range dhcpHandlers {
			if handler.IP == nil {
				continue
			}
			status := "badge-error"
			if handler.Started {
				status = "badge-success"
			}

			data.Servers = append(data.Servers, DHCPServer{
				TFTPIP: handler.IP.String(),
				Status: status,
				Leases: generateLeasesFromHandler(handler),
			})
		}
	}
	return data, nil
}

// generateLeasesFromHandler processes leases from a DHCP handler and sorts them by IP.
func generateLeasesFromHandler(handler *dhcp.DHCPHandler) []DHCPLease {
	sortedLeases := make([]dhcp.Lease, 0, len(handler.Leases))
	for _, lease := range handler.Leases {
		sortedLeases = append(sortedLeases, lease)
	}

	sort.Slice(sortedLeases, func(i, j int) bool {
		return sortedLeases[i].IP.String() < sortedLeases[j].IP.String()
	})

	leases := make([]DHCPLease, 0, len(sortedLeases))

	for _, lease := range sortedLeases {
		leases = append(leases, DHCPLease{
			IP:     lease.IP.String(),
			MAC:    lease.MAC,
			Static: lease.Reserved,
			Menu:   lease.Menu,
			IPMI:   lease.IPMI,
		})
	}
	return leases
}

func NewDHCPModal() map[string]any {
	return map[string]any{
		"Networks": getNetworkItems(),
	}
}

func NewReserveModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := GetBoltDHCPServer(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting DHCP server: %v", err), http.StatusInternalServerError)
		return nil, fmt.Errorf("error getting dhcp server for reserve modal: %s", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"tftpip": handler.IP.String(),
			"mac":    lease.MAC,
			"ip":     lease.IP.String(),
			"static": lease.Reserved,
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

func NewBootModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := GetBoltDHCPServer(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting DHCP server: %v", err), http.StatusInternalServerError)
		return nil, fmt.Errorf("error getting dhcp server for reserve modal: %s", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"tftpip":        CheckEmpty(handler.IP),
			"mac":           CheckEmpty(lease.MAC),
			"hostname":      CheckEmpty(lease.Menu.Hostname),
			"os":            CheckEmpty(lease.Menu.OS),
			"typeSelect":    CheckEmpty(lease.Menu.Template_Type),
			"template_name": CheckEmpty(lease.Menu.Template_Name),
			"ip":            CheckEmpty(lease.Menu.IP),
			"subnet":        CheckEmpty(lease.Menu.Subnet),
			"gateway":       CheckEmpty(lease.Menu.Gateway),
			"dns":           CheckEmpty(lease.Menu.DNS),
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

func NewIPMIModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := GetBoltDHCPServer(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting DHCP server: %v", err), http.StatusInternalServerError)
		return nil, fmt.Errorf("error getting dhcp server for reserve modal: %s", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"pxeboot":  CheckEmpty(lease.IPMI.Pxeboot),
			"reboot":   CheckEmpty(lease.IPMI.Reboot),
			"tftpip":   CheckEmpty(handler.IP),
			"mac":      CheckEmpty(lease.MAC),
			"ip":       CheckEmpty(lease.IPMI.IP),
			"username": CheckEmpty(lease.IPMI.Username),
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

// getNetworkItems collects IP addresses of active network interfaces.
func getNetworkItems() []string {
	ipItems := []string{}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ipItems
	}

	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || (i.Flags&net.FlagLoopback != 0 && i.Name != "lo") {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := getIPFromAddr(addr)
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				ipItems = append(ipItems, ip.String())
			}
		}
	}

	return ipItems
}

// getIPFromAddr extracts the IP address from a net.Addr interface.
func getIPFromAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

// SubmitDHCPServer handles the submission of new DHCP server configurations.
func SubmitDHCPServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	network, subnet, gateway, dns, startIP, err := extractAndValidateIPData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	numLeasesInt, err := strconv.Atoi(r.Form.Get("numLeases"))
	if err != nil || numLeasesInt < 1 {
		http.Error(w, "Invalid number of leases. Must be a positive integer.", http.StatusBadRequest)
		return
	}

	newDHCP := dhcp.NewDHCPHandler(network, subnet, gateway, dns, startIP, numLeasesInt)
	if err := newDHCP.Start(); err != nil {
		http.Error(w, "Failed to start DHCP server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("New DHCP server submitted: Network: %s, Subnet: %s, Start IP: %s, Leases: %d\n", network, subnet, startIP, numLeasesInt)
	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// extractAndValidateIPData validates and extracts IP address data from form values.
func extractAndValidateIPData(r *http.Request) (net.IP, net.IP, net.IP, net.IP, net.IP, error) {
	fields := []string{"network", "subnet", "gateway", "dns", "startIP"}
	ips := make([]net.IP, len(fields))

	for i, field := range fields {
		value := r.Form.Get(field)
		if value == "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("%s is required", field)
		}
		ips[i] = net.ParseIP(value)
		if ips[i] == nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid %s format", field)
		}
	}

	return ips[0], ips[1], ips[2], ips[3], ips[4], nil
}

// StartDHCPServer starts a DHCP server if not already running.
func StartDHCPServer(w http.ResponseWriter, r *http.Request) {
	d, err := GetBoltDHCPServer(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting DHCP server: %v", err), http.StatusInternalServerError)
		return
	}

	if d == nil {
		http.Error(w, "DHCP server not found", http.StatusNotFound)
		return
	}

	if d.Started {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("DHCP server is already started"))
	} else {
		if err := d.Start(); err != nil {
			http.Error(w, fmt.Sprintf("Error starting DHCP server: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		log.Printf("Started DHCP server: %s", d.IP)
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// StopDHCPServer stops an active DHCP server.
func StopDHCPServer(w http.ResponseWriter, r *http.Request) {
	d, err := GetBoltDHCPServer(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting DHCP server: %v", err), http.StatusInternalServerError)
		return
	}

	if d == nil {
		http.Error(w, "DHCP server not found", http.StatusNotFound)
		return
	}

	if !d.Started {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("DHCP server is already stopped"))
	} else {
		if err := d.Stop(); err != nil {
			log.Printf("Error stopping DHCP server: %v", err)
			http.Error(w, fmt.Sprintf("Error stopping DHCP server: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		log.Printf("Stopped DHCP server: %s", d.IP)
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// DeleteDHCPServer removes a DHCP server from the database.
func DeleteDHCPServer(w http.ResponseWriter, r *http.Request) {
	network, err := GetQueryParam(r, "network")
	if err != nil {
		log.Printf("Error getting network param: %v", err)
		http.Error(w, "Network parameter is required", http.StatusBadRequest)
		return
	}

	err = db.KV.DeleteKV(config.Defaults.DB.Bucket, []byte(network))
	if err != nil {
		log.Printf("Error deleting DHCP server: %v", err)
		http.Error(w, "Failed to delete DHCP server", http.StatusInternalServerError)
		return
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// GetBoltDHCPServer retrieves a DHCP server from the database by its network identifier.
func GetBoltDHCPServer(r *http.Request) (*dhcp.DHCPHandler, error) {
	network, err := GetQueryParam(r, "network")
	if err != nil {
		return nil, fmt.Errorf("error getting network param: %v", err)
	}

	data, err := db.KV.GetKV(config.Defaults.DB.Bucket, []byte(network))
	if err != nil {
		log.Printf("error getting DHCP handler from Bolt: %v", err)
		return nil, err
	}

	var dhcpHandler dhcp.DHCPHandler
	if err := json.Unmarshal(data, &dhcpHandler); err != nil {
		log.Printf("unable to unmarshal DHCP handler: %v", err)
		return nil, err
	}

	return &dhcpHandler, nil
}

// ReserveLease sets or updates a lease reservation for a given MAC and IP.
func ReserveLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := GetBoltDHCPServer(r)
	if err != nil {
		log.Printf("Error getting DHCP server: %v", err)
		http.Error(w, "Failed to get DHCP server", http.StatusInternalServerError)
		return
	}

	mac, err := GetQueryParam(r, "mac")
	if err != nil {
		log.Printf("Error getting MAC param: %v", err)
		http.Error(w, "MAC address parameter is required", http.StatusBadRequest)
		return
	}

	ip, err := GetQueryParam(r, "ip")
	if err != nil {
		log.Printf("Error getting IP param: %v", err)
		http.Error(w, "IP address parameter is required", http.StatusBadRequest)
		return
	}

	err = dhcpHandler.SetLeaseReservation(mac, ip, true)
	if err != nil {
		msg := fmt.Sprintf("Failed to reserve lease: %s", err.Error())
		log.Printf("Error reserving lease: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
}

// UnreserveLease removes a lease reservation for a given MAC and IP.
func UnreserveLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := GetBoltDHCPServer(r)
	if err != nil {
		log.Printf("Error getting DHCP server: %v", err)
		http.Error(w, "Failed to get DHCP server", http.StatusInternalServerError)
		return
	}

	mac, err := GetQueryParam(r, "mac")
	if err != nil {
		log.Printf("Error getting MAC param: %v", err)
		http.Error(w, "MAC address parameter is required", http.StatusBadRequest)
		return
	}

	ip, err := GetQueryParam(r, "ip")
	if err != nil {
		log.Printf("Error getting IP param: %v", err)
		http.Error(w, "IP address parameter is required", http.StatusBadRequest)
		return
	}

	err = dhcpHandler.SetLeaseReservation(mac, ip, false)
	if err != nil {
		log.Printf("Error unreserving lease: %v", err)
		http.Error(w, "Failed to unreserve lease", http.StatusInternalServerError)
		return
	}
}

// DeleteLease sets or updates a lease reservation for a given MAC and IP.
func DeleteLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := GetBoltDHCPServer(r)
	if err != nil {
		log.Printf("Error getting DHCP server: %v", err)
		http.Error(w, "Failed to get DHCP server", http.StatusInternalServerError)
		return
	}

	mac, err := GetQueryParam(r, "mac")
	if err != nil {
		log.Printf("Error getting MAC param: %v", err)
		http.Error(w, "MAC address parameter is required", http.StatusBadRequest)
		return
	}

	if _, ok := dhcpHandler.Leases[mac]; !ok {
		http.Error(w, "Lease not found", http.StatusNotFound)
		return
	}

	delete(dhcpHandler.Leases, mac)
	if err := dhcpHandler.UpdateDBState(); err != nil {
		http.Error(w, "Failed to delete lease", http.StatusInternalServerError)
		return
	}
}
