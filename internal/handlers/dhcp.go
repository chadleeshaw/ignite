package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"ignite/dhcp"
	"ignite/internal/errors"
	"ignite/internal/validation"
	"net"
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
func (h *Handlers) HandleDHCPPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()
	data, err := h.getDHCPData()
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_data", err))
		return
	}

	SetNoCacheHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	if err := templates["dhcp"].Execute(w, data); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
	}
}

// GetDHCPServers returns JSON data of all DHCP servers using structured data.
func (h *Handlers) GetDHCPServers(w http.ResponseWriter, r *http.Request) {
	data, err := h.getDHCPData()
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_data", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("encode_json", err))
	}
}

// getDHCPData retrieves and formats data about all DHCP servers for display.
func (h *Handlers) getDHCPData() (*DHCPPageData, error) {
	data := &DHCPPageData{
		Title:   "DHCP Leases",
		Servers: make([]DHCPServer, 0),
	}

	dhcpHandlers, err := h.getAllDHCPServers()
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP servers: %w", err)
	}

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
			Leases: h.generateLeasesFromHandler(handler),
		})
	}
	return data, nil
}

// getAllDHCPServers retrieves all DHCP server instances from the database.
func (h *Handlers) getAllDHCPServers() ([]*dhcp.DHCPHandler, error) {
	kv, err := h.DB.GetAllKV(h.GetDBBucket())
	if err != nil {
		return nil, fmt.Errorf("error getting DHCP servers from database: %w", err)
	}

	var dhcpServers []*dhcp.DHCPHandler
	for k, v := range kv {
		var dhcpServer dhcp.DHCPHandler
		if err := json.Unmarshal(v, &dhcpServer); err != nil {
			h.Logger.Warn("Failed to unmarshal DHCP server data",
				slog.String("key", k),
				slog.String("error", err.Error()))
			continue // Skip this item but continue with others
		}
		dhcpServers = append(dhcpServers, &dhcpServer)
	}

	return dhcpServers, nil
}

// generateLeasesFromHandler processes leases from a DHCP handler and sorts them by IP.
func (h *Handlers) generateLeasesFromHandler(handler *dhcp.DHCPHandler) []DHCPLease {
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

// OpenModalHandler handles modal opening requests with dependency injection
func (h *Handlers) OpenModalHandler(w http.ResponseWriter, r *http.Request) {
	template, err := getQueryParam(r, "template")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_template_param", err))
		return
	}

	templates := LoadTemplates()
	if t, ok := templates[template]; !ok {
		h.Logger.Warn("Template not found", slog.String("template", template))
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("template_not_found",
			fmt.Errorf("template %s not found", template)))
		return
	} else {
		var data map[string]any
		var err error

		switch template {
		case "dhcpmodal":
			data = h.newDHCPModal()
		case "reservemodal":
			data, err = h.newReserveModal(w, r)
			if err != nil {
				errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("create_reserve_modal", err))
				return
			}
		case "bootmodal":
			data, err = h.newBootModal(w, r)
			if err != nil {
				errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("create_boot_modal", err))
				return
			}
		case "ipmimodal":
			data, err = h.newIPMIModal(w, r)
			if err != nil {
				errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("create_ipmi_modal", err))
				return
			}
		case "upload":
			data = h.newUploadModal(w, r)
		case "provtempmodal":
		case "provconfigmodal":
		case "provsaveasmodal":
		default:
			h.Logger.Warn("Unhandled template type", slog.String("template", template))
			errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("unhandled_template",
				fmt.Errorf("unhandled template type: %s", template)))
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if err := t.Execute(w, data); err != nil {
			errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
		}
	}
}

// SubmitDHCPServer handles the submission of new DHCP server configurations.
func (h *Handlers) SubmitDHCPServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("parse_form", err))
		return
	}

	network, subnet, gateway, dns, startIP, err := h.extractAndValidateIPData(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_ip_data", err))
		return
	}

	numLeasesStr := r.Form.Get("numLeases")
	if err := validation.ValidateRequired("numLeases", numLeasesStr); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_num_leases", err))
		return
	}

	numLeasesInt, err := strconv.Atoi(numLeasesStr)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("parse_num_leases", err))
		return
	}

	if err := validation.ValidateLeaseRange(numLeasesInt); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_lease_range", err))
		return
	}

	newDHCP := dhcp.NewDHCPHandler(network, subnet, gateway, dns, startIP, numLeasesInt)
	if err := newDHCP.Start(); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("start_dhcp_server", err))
		return
	}

	h.Logger.Info("New DHCP server created",
		slog.String("tftpip", network.String()),
		slog.String("subnet", subnet.String()),
		slog.String("start_ip", startIP.String()),
		slog.Int("leases", numLeasesInt))

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// StartDHCPServer starts a DHCP server if not already running.
func (h *Handlers) StartDHCPServer(w http.ResponseWriter, r *http.Request) {
	d, err := h.getBoltDHCPServer(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_server", err))
		return
	}

	if d == nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("dhcp_server_not_found",
			fmt.Errorf("DHCP server not found")))
		return
	}

	if d.Started {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("DHCP server is already started"))
	} else {
		if err := d.Start(); err != nil {
			errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("start_dhcp_server", err))
			return
		}
		w.WriteHeader(http.StatusOK)
		h.Logger.Info("Started DHCP server", slog.String("ip", d.IP.String()))
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// StopDHCPServer stops an active DHCP server.
func (h *Handlers) StopDHCPServer(w http.ResponseWriter, r *http.Request) {
	d, err := h.getBoltDHCPServer(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_server", err))
		return
	}

	if d == nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("dhcp_server_not_found",
			fmt.Errorf("DHCP server not found")))
		return
	}

	if !d.Started {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("DHCP server is already stopped"))
	} else {
		if err := d.Stop(); err != nil {
			errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("stop_dhcp_server", err))
			return
		}
		w.WriteHeader(http.StatusOK)
		h.Logger.Info("Stopped DHCP server", slog.String("ip", d.IP.String()))
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// DeleteDHCPServer removes a DHCP server from the database.
func (h *Handlers) DeleteDHCPServer(w http.ResponseWriter, r *http.Request) {
	network, err := getQueryParam(r, "tftpip")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_network_param", err))
		return
	}

	if err := validation.ValidateIP(network); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_network", err))
		return
	}

	if err := h.DB.DeleteKV(h.GetDBBucket(), []byte(network)); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("delete_dhcp_server", err))
		return
	}

	h.Logger.Info("Deleted DHCP server", slog.String("tftpip", network))
	SetNoCacheHeaders(w)
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// ReserveLease sets or updates a lease reservation for a given MAC and IP.
func (h *Handlers) ReserveLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := h.getBoltDHCPServer(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_server", err))
		return
	}

	mac, err := getQueryParam(r, "mac")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_mac_param", err))
		return
	}

	if err := validation.ValidateMAC(mac); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_mac", err))
		return
	}

	ip, err := getQueryParam(r, "ip")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_ip_param", err))
		return
	}

	if err := validation.ValidateIP(ip); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_ip", err))
		return
	}

	if err := dhcpHandler.SetLeaseReservation(mac, ip, true); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("reserve_lease", err))
		return
	}

	h.Logger.Info("Reserved lease", slog.String("mac", mac), slog.String("ip", ip))
}

// UnreserveLease removes a lease reservation for a given MAC and IP.
func (h *Handlers) UnreserveLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := h.getBoltDHCPServer(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_server", err))
		return
	}

	mac, err := getQueryParam(r, "mac")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_mac_param", err))
		return
	}

	if err := validation.ValidateMAC(mac); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_mac", err))
		return
	}

	ip, err := getQueryParam(r, "ip")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_ip_param", err))
		return
	}

	if err := validation.ValidateIP(ip); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_ip", err))
		return
	}

	if err := dhcpHandler.SetLeaseReservation(mac, ip, false); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("unreserve_lease", err))
		return
	}

	h.Logger.Info("Unreserved lease", slog.String("mac", mac), slog.String("ip", ip))
}

// DeleteLease deletes a lease for a given MAC.
func (h *Handlers) DeleteLease(w http.ResponseWriter, r *http.Request) {
	dhcpHandler, err := h.getBoltDHCPServer(r)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("get_dhcp_server", err))
		return
	}

	mac, err := getQueryParam(r, "mac")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("get_mac_param", err))
		return
	}

	if err := validation.ValidateMAC(mac); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_mac", err))
		return
	}

	delete(dhcpHandler.Leases, mac)
	dhcpHandler.UpdateDBState()

	h.Logger.Info("Deleted lease", slog.String("mac", mac))
}

// CloseModalHandler closes the modal by returning an empty div.
func (h *Handlers) CloseModalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<div id=\"modal-content\"></div>"))
}

func (h *Handlers) newDHCPModal() map[string]any {
	return map[string]any{}
}

func (h *Handlers) newReserveModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	mac, err := getQueryParam(r, "mac")
	if err != nil {
		return nil, err
	}

	ip, err := getQueryParam(r, "ip")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"mac": mac,
		"ip":  ip,
	}, nil
}

func (h *Handlers) newBootModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	mac, err := getQueryParam(r, "mac")
	if err != nil {
		return nil, err
	}

	ip, err := getQueryParam(r, "ip")
	if err != nil {
		return nil, err
	}

	tftpip, err := getQueryParam(r, "tftpip")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"mac":    mac,
		"ip":     ip,
		"tftpip": tftpip,
	}, nil
}

func (h *Handlers) newIPMIModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := h.getBoltDHCPServer(r)
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

func (h *Handlers) newUploadModal(w http.ResponseWriter, r *http.Request) map[string]any {
	return map[string]any{}
}

func (h *Handlers) extractAndValidateIPData(r *http.Request) (net.IP, net.IP, net.IP, net.IP, net.IP, error) {
	ipFields := map[string]string{
		"tftpip":  r.Form.Get("tftpip"),
		"subnet":  r.Form.Get("subnet"),
		"gateway": r.Form.Get("gateway"),
		"dns":     r.Form.Get("dns"),
		"startIP": r.Form.Get("startIP"),
	}

	parsedIPs := make(map[string]net.IP)
	for key, val := range ipFields {
		if err := validation.ValidateIP(val); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid IP address for %s: %w", key, err)
		}
		parsedIPs[key] = net.ParseIP(val)
	}

	return parsedIPs["tftpip"], parsedIPs["subnet"], parsedIPs["gateway"], parsedIPs["dns"], parsedIPs["startIP"], nil
}

func (h *Handlers) getBoltDHCPServer(r *http.Request) (*dhcp.DHCPHandler, error) {
	network, err := getQueryParam(r, "tftpip")
	if err != nil {
		return nil, fmt.Errorf("could not get network from query params: %w", err)
	}

	dhcpData, err := h.DB.GetKV(h.GetDBBucket(), []byte(network))
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP handler from database: %w", err)
	}

	var dhcpServer dhcp.DHCPHandler
	if err := json.Unmarshal(dhcpData, &dhcpServer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DHCP server data: %w", err)
	}

	return &dhcpServer, nil
}
