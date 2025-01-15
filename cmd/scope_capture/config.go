package main

var config = configT{
	AppTitle:    "RIGOL Scope Capture",
	AppName:     "scope_capture",
	DefaultIp:   "169.254.247.73",
	DefaultPort: 5555,
	// Hostname is assigned at runtime
	Hostname: "",
}

type configT struct {
	AppTitle    string
	AppName     string
	DefaultPort int
	DefaultIp   string
	Hostname    string
}
