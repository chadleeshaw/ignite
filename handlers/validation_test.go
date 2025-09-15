package handlers

import (
	"testing"
)

func TestIPValidator_ValidateIPAddress(t *testing.T) {
	validator := NewIPValidator()

	tests := []struct {
		name      string
		ip        string
		expectErr bool
	}{
		{"Valid IPv4", "192.168.1.1", false},
		{"Valid IPv4 with zero", "10.0.0.1", false},
		{"Invalid format", "192.168.1", true},
		{"Empty string", "", true},
		{"Invalid characters", "192.168.1.a", true},
		{"IPv6 address", "2001:db8::1", true}, // We only support IPv4
		{"Out of range", "256.256.256.256", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateIPAddress(tt.ip)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateIPAddress(%s) error = %v, expectErr %v", tt.ip, err, tt.expectErr)
			}
		})
	}
}

func TestIPValidator_ValidateIPRange(t *testing.T) {
	validator := NewIPValidator()

	tests := []struct {
		name      string
		rangeStr  string
		expectErr bool
	}{
		{"Valid range", "192.168.1.10-192.168.1.20", false},
		{"Single IP range", "192.168.1.10-192.168.1.10", false},
		{"Invalid format", "192.168.1.10", true},
		{"Reversed range", "192.168.1.20-192.168.1.10", true},
		{"Invalid start IP", "192.168.1-192.168.1.20", true},
		{"Invalid end IP", "192.168.1.10-192.168.1", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateIPRange(tt.rangeStr)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateIPRange(%s) error = %v, expectErr %v", tt.rangeStr, err, tt.expectErr)
			}
		})
	}
}

func TestIPValidator_ValidateSubnet(t *testing.T) {
	validator := NewIPValidator()

	tests := []struct {
		name      string
		subnet    string
		expectErr bool
	}{
		{"Valid CIDR", "192.168.1.0/24", false},
		{"Valid CIDR /16", "10.0.0.0/16", false},
		{"Valid CIDR /8", "172.16.0.0/8", false},
		{"Invalid CIDR", "192.168.1.0/33", true},
		{"No CIDR", "192.168.1.0", true},
		{"Invalid IP", "192.168.1/24", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSubnet(tt.subnet)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateSubnet(%s) error = %v, expectErr %v", tt.subnet, err, tt.expectErr)
			}
		})
	}
}

func TestIPValidator_ValidateMACAddress(t *testing.T) {
	validator := NewIPValidator()

	tests := []struct {
		name      string
		mac       string
		expectErr bool
	}{
		{"Valid MAC colon", "00:11:22:33:44:55", false},
		{"Valid MAC hyphen", "00-11-22-33-44-55", false},
		{"Valid MAC uppercase", "AA:BB:CC:DD:EE:FF", false},
		{"Valid MAC lowercase", "aa:bb:cc:dd:ee:ff", false},
		{"Invalid format", "00:11:22:33:44", true},
		{"Invalid characters", "00:11:22:33:44:GG", true},
		{"Mixed separators", "00:11-22:33:44:55", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateMACAddress(tt.mac)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateMACAddress(%s) error = %v, expectErr %v", tt.mac, err, tt.expectErr)
			}
		})
	}
}

func TestDHCPConfigValidator_ValidateDHCPConfig(t *testing.T) {
	validator := NewDHCPConfigValidator()

	tests := []struct {
		name      string
		config    map[string]string
		expectErr bool
	}{
		{
			name: "Valid config",
			config: map[string]string{
				"subnet": "192.168.1.0/24",
				"range":  "192.168.1.10-192.168.1.20",
				"router": "192.168.1.1",
				"dns":    "8.8.8.8",
			},
			expectErr: false,
		},
		{
			name: "Missing subnet",
			config: map[string]string{
				"range":  "192.168.1.10-192.168.1.20",
				"router": "192.168.1.1",
			},
			expectErr: true,
		},
		{
			name: "Invalid subnet",
			config: map[string]string{
				"subnet": "192.168.1.0/33",
				"range":  "192.168.1.10-192.168.1.20",
			},
			expectErr: true,
		},
		{
			name: "Range outside subnet",
			config: map[string]string{
				"subnet": "192.168.1.0/24",
				"range":  "192.168.2.10-192.168.2.20",
			},
			expectErr: true,
		},
		{
			name: "Router outside subnet",
			config: map[string]string{
				"subnet": "192.168.1.0/24",
				"range":  "192.168.1.10-192.168.1.20",
				"router": "192.168.2.1",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateDHCPConfig(tt.config)
			hasErr := errors.HasErrors()
			if hasErr != tt.expectErr {
				t.Errorf("ValidateDHCPConfig(%+v) hasErrors = %v, expectErr %v", tt.config, hasErr, tt.expectErr)
				if hasErr {
					t.Logf("Validation errors: %+v", errors)
				}
			}
		})
	}
}

func TestValidationErrors(t *testing.T) {
	errors := make(ValidationErrors)

	// Test empty errors
	if errors.HasErrors() {
		t.Error("Expected no errors for empty ValidationErrors")
	}

	// Add some errors
	errors.Add("field1", "error1")
	errors.Add("field1", "error2")
	errors.Add("field2", "error3")

	if !errors.HasErrors() {
		t.Error("Expected errors after adding")
	}

	// Check field1 has 2 errors
	if len(errors["field1"]) != 2 {
		t.Errorf("Expected 2 errors for field1, got %d", len(errors["field1"]))
	}

	// Check field2 has 1 error
	if len(errors["field2"]) != 1 {
		t.Errorf("Expected 1 error for field2, got %d", len(errors["field2"]))
	}

	// Test ToAppError
	appErr := errors.ToAppError()
	if appErr == nil {
		t.Error("Expected AppError from ToAppError")
		return
	}

	if appErr.Type != ErrorTypeValidation {
		t.Errorf("Expected validation error type, got %s", appErr.Type)
	}
}
