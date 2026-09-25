package protocol

import "slices"

// DeviceKind says what sort of client a device is. The list is the same as the check on the
// devices table, so the database and the API cannot disagree.
type DeviceKind string

const (
	// DeviceKindWeb is a browser.
	DeviceKindWeb DeviceKind = "web"
	// DeviceKindDesktop is the desktop app.
	DeviceKindDesktop DeviceKind = "desktop"
	// DeviceKindMobile is a phone or tablet paired over the tailnet.
	DeviceKindMobile DeviceKind = "mobile"
	// DeviceKindCLI is the `marshal` command line tool. It uses the owner token file.
	DeviceKindCLI DeviceKind = "cli"
	// DeviceKindDev is the dev token, which only a dev daemon accepts and only from this machine.
	DeviceKindDev DeviceKind = "dev"
)

// DeviceKindValues lists every device kind.
func DeviceKindValues() []DeviceKind {
	return []DeviceKind{DeviceKindWeb, DeviceKindDesktop, DeviceKindMobile, DeviceKindCLI, DeviceKindDev}
}

// Valid reports whether k is a device kind.
func (k DeviceKind) Valid() bool { return slices.Contains(DeviceKindValues(), k) }

// WhoAmI is the answer to GET /v1/auth/whoami. A client calls it to check that its token still
// works and to learn which device and user the token belongs to.
type WhoAmI struct {
	// DeviceID is the opaque id of the device the token belongs to.
	DeviceID string `json:"deviceId"`
	// DeviceKind is what sort of client that device is.
	DeviceKind DeviceKind `json:"deviceKind"`
	// UserID is the opaque id of the user who owns the device.
	UserID string `json:"userId"`
	// Mode is "normal" or "dev", the same word as in Health.
	Mode string `json:"mode"`
	// ServerTime is the daemon's clock when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}
