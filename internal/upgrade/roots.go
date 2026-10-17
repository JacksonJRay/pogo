package upgrade

// Mozilla's root set is used only when the host trust store is empty.
// A non-empty system pool, SSL_CERT_FILE, or SSL_CERT_DIR stays in front of it.
import _ "golang.org/x/crypto/x509roots/fallback"
