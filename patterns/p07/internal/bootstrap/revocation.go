package bootstrap

import "sync"

// RevocationList is a thread-safe in-memory deny-list of revoked device Common Names.
// Because tokens are short-lived, revocation takes effect within one TTL window
// without requiring OCSP or CRL infrastructure.
type RevocationList struct {
	mu      sync.RWMutex
	revoked map[string]struct{}
}

// NewRevocationList creates a RevocationList pre-populated with the given CNs.
func NewRevocationList(initial []string) *RevocationList {
	r := &RevocationList{revoked: make(map[string]struct{}, len(initial))}
	for _, cn := range initial {
		r.revoked[cn] = struct{}{}
	}
	return r
}

// Revoke adds a device CN to the deny-list. Subsequent bootstrap calls from
// devices with this CN will receive HTTP 403.
func (r *RevocationList) Revoke(cn string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revoked[cn] = struct{}{}
}

// IsRevoked reports whether the given CN is on the deny-list.
func (r *RevocationList) IsRevoked(cn string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.revoked[cn]
	return ok
}
