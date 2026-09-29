package protocol

import "time"

// Device is one paired client as the profile's paired-devices list shows it (B9.2, build-plan
// 9.2). It is the row the `devices` table already holds, minus the token hash: a token never
// leaves the call that made it.
type Device struct {
	// ID is the device's opaque id.
	ID string `json:"id"`
	// Name is what the person called the device when it was paired.
	Name string `json:"name"`
	// Kind is what sort of client it is. A paired phone or tablet is `mobile`, a paired browser
	// is `web`. The `cli` and `dev` kinds belong to the owner token and the dev token and are
	// never created by pairing.
	Kind DeviceKind `json:"kind"`
	// PairedAt is when the device was given its token.
	PairedAt Timestamp `json:"pairedAt"`
	// LastSeenAt is the last time the device called, or null when it has not called yet.
	LastSeenAt *Timestamp `json:"lastSeenAt" tstype:"Timestamp | null"`
	// Revoked is true for a device whose token no longer signs in. A revoked device is kept in
	// the list so the screen can say it was removed rather than quietly dropping the row.
	Revoked bool `json:"revoked"`
}

// DeviceList is the answer to GET /v1/devices.
type DeviceList struct {
	// Devices is every device of the person, oldest first, revoked ones included.
	Devices []Device `json:"devices"`
	// ServerTime is the daemon's clock when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewDeviceList makes an answer stamped with the daemon's time.
func NewDeviceList(devices []Device, now time.Time) DeviceList {
	out := make([]Device, len(devices))
	copy(out, devices)
	return DeviceList{Devices: out, ServerTime: NewTimestamp(now)}
}

// PairingCode is the answer to POST /v1/devices/pairing-code: the short code the person types on
// the device being paired, and the moment it stops working. The code is shown on the device that
// already has access and is never sent to the device being paired except by the person reading it
// off the screen.
type PairingCode struct {
	// Code is the short code, grouped for reading, such as "7QX-2LD".
	Code string `json:"code"`
	// ExpiresAt is when the code stops working.
	ExpiresAt Timestamp `json:"expiresAt"`
	// ServerTime is the daemon's clock when the code was made.
	ServerTime Timestamp `json:"serverTime"`
}

// PairDeviceRequest is the body of POST /v1/devices/pair. It is the one route besides health and
// the webhooks that takes no token: a device being paired has no token yet, which is the whole
// point, so the short-lived code is what authorizes it.
type PairDeviceRequest struct {
	// Code is the code read off the paired device's screen.
	Code string `json:"code"`
	// Name is what to call this device in the paired-devices list, such as "Blair's iPhone".
	Name string `json:"name"`
	// Kind is what sort of client it is: `mobile` or `web`. The `cli` and `dev` kinds cannot be
	// paired, because they belong to tokens the daemon writes to disk itself.
	Kind DeviceKind `json:"kind"`
}

// PairDeviceResponse is the answer to POST /v1/devices/pair: the token the new device stores and
// signs in with from then on, and the row it now owns. The token is sent once, in this answer, and
// the database keeps only its hash.
type PairDeviceResponse struct {
	// Token is the device's token. It is never sent again.
	Token string `json:"token"`
	// Device is the row that was just made.
	Device Device `json:"device"`
	// ServerTime is the daemon's clock when the device was paired.
	ServerTime Timestamp `json:"serverTime"`
}
