// Package certstore is a local, cgo-free stand-in for github.com/tailscale/certstore, the
// upstream module it replaces (see the replace directive in the daemon's go.mod).
//
// Why it exists: on macOS the upstream store talks to the Keychain through C, so linking it needs
// cgo. This project's daemon is deliberately cgo-free - docs/progress-tracker.md records that
// "this Mac's Command Line Tools cannot link cgo programs that use system frameworks" - and
// tailscale.com reaches this package through control/controlclient on darwin and windows. Without
// a replacement, `go build ./...` in daemon/ fails at the link step of marshald, and the daemon
// can no longer build at all.
//
// What it gives up: nothing Marshal uses. The store backs only one thing in tailscale.com - a
// machine certificate supplied by an administrator's device policy, used to sign a node's
// registration request (control/controlclient.signRegisterRequest). Marshal does not configure
// device policy, so that path is never taken; node identity, MagicDNS, the tailnet listener, and
// Funnel's certificate all come from the Tailscale control plane instead. When the path is
// somehow taken, this package answers ErrUnavailable and tailscale.com reports that it could not
// find a machine identity - it never signs with something it did not find.
//
// The API below is the same shape as the upstream package's, so tailscale.com compiles against it
// unchanged. Method sets must match exactly: an interface that lost a method would fail to build,
// and one that gained one would silently allow a call upstream never makes.
package certstore

import (
	"crypto"
	"crypto/x509"
	"errors"
)

// ErrUnavailable says this build has no platform certificate store. It is what every call in this
// package answers with, because the whole point of the package is that the platform store cannot
// be reached from a cgo-free build.
var ErrUnavailable = errors.New("the platform certificate store is not available in a cgo-free build")

// ErrUnsupportedHash is returned by a Signer's Sign when the provided hash algorithm is not
// supported. No signer is ever made by this package, so it exists only to keep the API identical
// to the upstream one.
var ErrUnsupportedHash = errors.New("unsupported hash algorithm")

// StoreLocation says which store to look for certificates in.
type StoreLocation int

const (
	// User is the user-scoped certificate store: "CURRENT_USER" on Windows, "login" on macOS.
	User StoreLocation = iota
	// System is the system-scoped certificate store: "LOCAL_MACHINE" on Windows, "System" on
	// macOS.
	System
)

// StorePermission says how the store may be used once it is open.
type StorePermission int

const (
	// ReadOnly allows reading and using certificates.
	ReadOnly StorePermission = iota
	// ReadWrite allows importing certificates.
	ReadWrite
)

// Open would open the system's certificate store. In this build it always answers
// ErrUnavailable: the upstream store needs cgo, and this daemon builds without it.
func Open(location StoreLocation, permissions ...StorePermission) (Store, error) {
	return nil, errors.Join(ErrUnavailable,
		errors.New("Marshal builds without cgo, so the Keychain and the Windows certificate store are not reachable"))
}

// Store is the system's certificate store.
type Store interface {
	// Identities gets a list of identities from the store.
	Identities() ([]Identity, error)
	// Import imports a PKCS#12 (PFX) blob containing a certificate and private key.
	Import(data []byte, password string) error
	// Close closes the store.
	Close()
}

// Identity is an X.509 certificate and its corresponding private key.
type Identity interface {
	// Certificate gets the identity's certificate.
	Certificate() (*x509.Certificate, error)
	// CertificateChain attempts to get the identity's full certificate chain.
	CertificateChain() ([]*x509.Certificate, error)
	// Signer gets a crypto.Signer that uses the identity's private key.
	Signer() (crypto.Signer, error)
	// Delete deletes this identity from the system.
	Delete() error
	// Close releases any manually managed memory held by the Identity.
	Close()
}
