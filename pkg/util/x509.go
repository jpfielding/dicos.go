package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"io"
	"strings"
)

// OID definitions for name components, as pkix.Name.String() doesn't format
// in the same order as OpenSSL.
var (
	OidCountry            = asn1.ObjectIdentifier{2, 5, 4, 6}
	OidOrganization       = asn1.ObjectIdentifier{2, 5, 4, 10}
	OidOrganizationalUnit = asn1.ObjectIdentifier{2, 5, 4, 11}
	OidLocality           = asn1.ObjectIdentifier{2, 5, 4, 7}
	OidProvince           = asn1.ObjectIdentifier{2, 5, 4, 8}
	OidStreetAddress      = asn1.ObjectIdentifier{2, 5, 4, 9}
	OidPostalCode         = asn1.ObjectIdentifier{2, 5, 4, 17}
	OidCommonName         = asn1.ObjectIdentifier{2, 5, 4, 3}
)

// keyUsageNameMap maps x509.KeyUsage constants to their string representations.
var keyUsageNameMap = map[x509.KeyUsage]string{
	x509.KeyUsageDigitalSignature:  "Digital Signature",
	x509.KeyUsageContentCommitment: "Content Commitment",
	x509.KeyUsageKeyEncipherment:   "Key Encipherment",
	x509.KeyUsageDataEncipherment:  "Data Encipherment",
	x509.KeyUsageKeyAgreement:      "Key Agreement",
	x509.KeyUsageCertSign:          "Certificate Sign",
	x509.KeyUsageCRLSign:           "CRL Sign",
	x509.KeyUsageEncipherOnly:      "Encipher Only",
	x509.KeyUsageDecipherOnly:      "Decipher Only",
}

// extKeyUsageNameMap maps x509.ExtKeyUsage constants to their string representations.
var extKeyUsageNameMap = map[x509.ExtKeyUsage]string{
	x509.ExtKeyUsageAny:                            "Any",
	x509.ExtKeyUsageServerAuth:                     "TLS Web Server Authentication",
	x509.ExtKeyUsageClientAuth:                     "TLS Web Client Authentication",
	x509.ExtKeyUsageCodeSigning:                    "Code Signing",
	x509.ExtKeyUsageEmailProtection:                "E-mail Protection",
	x509.ExtKeyUsageIPSECEndSystem:                 "IPSEC End System",
	x509.ExtKeyUsageIPSECTunnel:                    "IPSEC Tunnel",
	x509.ExtKeyUsageIPSECUser:                      "IPSEC User",
	x509.ExtKeyUsageTimeStamping:                   "Time Stamping",
	x509.ExtKeyUsageOCSPSigning:                    "OCSP Signing",
	x509.ExtKeyUsageMicrosoftServerGatedCrypto:     "Microsoft Server Gated Crypto",
	x509.ExtKeyUsageNetscapeServerGatedCrypto:      "Netscape Server Gated Crypto",
	x509.ExtKeyUsageMicrosoftCommercialCodeSigning: "Microsoft Commercial Code Signing",
	x509.ExtKeyUsageMicrosoftKernelCodeSigning:     "Microsoft Kernel Code Signing",
}

// PrettyPrintCert takes an *x509.Certificate and returns a formatted string
// similar to the output of `openssl x509 -text -noout`.
func PrettyPrintCert(cert *x509.Certificate) (string, error) {
	var b strings.Builder

	b.WriteString("Certificate:\n")
	b.WriteString("    Data:\n")
	fmt.Fprintf(&b, "        Version: %d (0x%x)\n", cert.Version, cert.Version-1)
	fmt.Fprintf(&b, "        Serial Number:\n            %s\n", cert.SerialNumber)

	b.WriteString("    Signature Algorithm: ")
	b.WriteString(cert.SignatureAlgorithm.String())
	b.WriteString("\n")

	fmt.Fprintf(&b, "        Issuer: %s\n", formatName(cert.Issuer))
	fmt.Fprintf(&b, "        Validity\n")
	fmt.Fprintf(&b, "            Not Before: %s\n", cert.NotBefore.Format("Jan 2 15:04:05 2006 MST"))
	fmt.Fprintf(&b, "            Not After : %s\n", cert.NotAfter.Format("Jan 2 15:04:05 2006 MST"))
	fmt.Fprintf(&b, "        Subject: %s\n", formatName(cert.Subject))

	b.WriteString("        Subject Public Key Info:\n")
	fmt.Fprintf(&b, "            Public Key Algorithm: %s\n", cert.PublicKeyAlgorithm.String())

	// Public key printing
	if err := formatPublicKey(&b, cert.PublicKey); err != nil {
		return "", err
	}

	// Extensions
	if len(cert.Extensions) > 0 {
		b.WriteString("        X509v3 extensions:\n")
		// Map to track handled extensions to avoid double printing from dedicated fields
		handledOIDs := make(map[string]bool)

		if cert.KeyUsage != 0 {
			oid := "2.5.29.15" // OID for Key Usage
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Key Usage: %s\n", formatCritical(critical))
			b.WriteString(indent(formatKeyUsage(cert.KeyUsage), 16))
			b.WriteString("\n")
			handledOIDs[oid] = true
		}

		if len(cert.ExtKeyUsage) > 0 {
			oid := "2.5.29.37" // OID for Extended Key Usage
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Extended Key Usage: %s\n", formatCritical(critical))
			b.WriteString(indent(formatExtKeyUsage(cert.ExtKeyUsage), 16))
			b.WriteString("\n")
			handledOIDs[oid] = true
		}

		if cert.BasicConstraintsValid {
			oid := "2.5.29.19" // OID for Basic Constraints
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Basic Constraints: %s\n", formatCritical(critical))
			fmt.Fprintf(&b, "                CA:%s\n", strings.ToUpper(fmt.Sprintf("%v", cert.IsCA)))
			if cert.MaxPathLenZero {
				fmt.Fprint(&b, "                pathlen:0\n")
			} else if cert.MaxPathLen > 0 {
				fmt.Fprintf(&b, "                pathlen:%d\n", cert.MaxPathLen)
			}
			handledOIDs[oid] = true
		}

		if len(cert.SubjectKeyId) > 0 {
			oid := "2.5.29.14" // OID for Subject Key Identifier
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Subject Key Identifier: %s\n", formatCritical(critical))
			fmt.Fprintf(&b, "                %s\n", formatHexWithColons(cert.SubjectKeyId))
			handledOIDs[oid] = true
		}

		if len(cert.AuthorityKeyId) > 0 {
			oid := "2.5.29.35" // OID for Authority Key Identifier
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Authority Key Identifier: %s\n", formatCritical(critical))
			fmt.Fprintf(&b, "                keyid:%s\n", formatHexWithColons(cert.AuthorityKeyId))
			handledOIDs[oid] = true
		}

		if len(cert.DNSNames) > 0 || len(cert.EmailAddresses) > 0 || len(cert.IPAddresses) > 0 {
			oid := "2.5.29.17" // OID for Subject Alternative Name
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            X509v3 Subject Alternative Name: %s\n", formatCritical(critical))
			var sans []string
			for _, dns := range cert.DNSNames {
				sans = append(sans, "DNS:"+dns)
			}
			for _, email := range cert.EmailAddresses {
				sans = append(sans, "email:"+email)
			}
			for _, ip := range cert.IPAddresses {
				sans = append(sans, "IP Address:"+ip.String())
			}
			b.WriteString(indent(strings.Join(sans, ", "), 16))
			b.WriteString("\n")
			handledOIDs[oid] = true
		}

		if len(cert.OCSPServer) > 0 || len(cert.IssuingCertificateURL) > 0 {
			oid := "1.3.6.1.5.5.7.1.1" // OID for Authority Information Access
			critical := isExtensionCritical(cert.Extensions, oid)
			fmt.Fprintf(&b, "            Authority Information Access: %s\n", formatCritical(critical))
			for _, ocsp := range cert.OCSPServer {
				fmt.Fprintf(&b, "                OCSP - URI:%s\n", ocsp)
			}
			for _, issuer := range cert.IssuingCertificateURL {
				fmt.Fprintf(&b, "                CA Issuers - URI:%s\n", issuer)
			}
			handledOIDs[oid] = true
		}

		// Print any other extensions not handled above
		for _, ext := range cert.Extensions {
			if !handledOIDs[ext.Id.String()] {
				fmt.Fprintf(&b, "            %s: %s\n", ext.Id.String(), formatCritical(ext.Critical))
				// Just print the raw value for unhandled extensions
				fmt.Fprintf(&b, "                %s\n", formatHexWithColons(ext.Value))
			}
		}
	}

	b.WriteString("    Signature Algorithm: ")
	b.WriteString(cert.SignatureAlgorithm.String())
	b.WriteString("\n")
	b.WriteString(indent(formatHexWithColons(cert.Signature), 9))
	b.WriteString("\n")

	return b.String(), nil
}

// formatName formats a pkix.Name into a string like OpenSSL does.
func formatName(name pkix.Name) string {
	var parts []string
	// The order is significant for mimicking OpenSSL's output.
	nameMap := map[string]string{
		OidCountry.String():            "C",
		OidProvince.String():           "ST",
		OidLocality.String():           "L",
		OidOrganization.String():       "O",
		OidOrganizationalUnit.String(): "OU",
		OidCommonName.String():         "CN",
	}

	for _, nameType := range []asn1.ObjectIdentifier{
		OidCountry, OidProvince, OidLocality, OidOrganization, OidOrganizationalUnit, OidCommonName,
	} {
		for _, namePart := range name.Names {
			if namePart.Type.Equal(nameType) {
				shortName, ok := nameMap[namePart.Type.String()]
				if !ok {
					shortName = namePart.Type.String()
				}
				parts = append(parts, fmt.Sprintf("%s=%s", shortName, namePart.Value))
			}
		}
	}
	return strings.Join(parts, ", ")
}

// formatPublicKey handles different types of public keys.
func formatPublicKey(w io.Writer, pub interface{}) error {
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		fmt.Fprintf(w, "                Public-Key: (%d bit)\n", pub.N.BitLen())
		// OpenSSL formats modulus with newlines and indentation.
		modulus := fmt.Sprintf("%x", pub.N)
		fmt.Fprintf(w, "                Modulus:\n")
		fmt.Fprint(w, indent(formatHexWithColons(modulus), 20))
		fmt.Fprintf(w, "\n                Exponent: %d (0x%x)\n", pub.E, pub.E)

	case *ecdsa.PublicKey:
		fmt.Fprintf(w, "                Public-Key: (%d bit)\n", pub.Curve.Params().BitSize)
		// Format public key bytes (uncompressed format)
		pubBytes := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
		fmt.Fprint(w, "                pub:\n")
		fmt.Fprint(w, indent(formatHexWithColons(pubBytes), 20))
		fmt.Fprintf(w, "\n                NIST CURVE: %s\n", pub.Curve.Params().Name)

	default:
		return fmt.Errorf("unsupported public key type: %T", pub)
	}
	return nil
}

// formatKeyUsage converts a KeyUsage bitmask to a comma-separated string.
func formatKeyUsage(ku x509.KeyUsage) string {
	var parts []string
	for usage, name := range keyUsageNameMap {
		if ku&usage != 0 {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}

// formatExtKeyUsage converts a slice of ExtKeyUsage to a comma-separated string.
func formatExtKeyUsage(ekus []x509.ExtKeyUsage) string {
	var parts []string
	for _, eku := range ekus {
		name, ok := extKeyUsageNameMap[eku]
		if !ok {
			name = "Unknown"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

// formatHexWithColons formats a byte slice or hex string into a hex string with colons.
func formatHexWithColons(data interface{}) string {
	var hexStr string
	switch v := data.(type) {
	case []byte:
		hexStr = fmt.Sprintf("%x", v)
	case string:
		hexStr = v
	default:
		return ""
	}

	var formatted strings.Builder
	for i, r := range hexStr {
		if i > 0 && i%2 == 0 {
			formatted.WriteString(":")
		}
		formatted.WriteRune(r)
	}
	// Wrap lines for long hex strings
	return wrapString(formatted.String(), 45) // 15 hex bytes per line (15*3-1 chars)
}

// wrapString wraps a string at a given line length.
func wrapString(s string, lineLen int) string {
	if len(s) <= lineLen {
		return s
	}
	var result []string
	for len(s) > 0 {
		cut := lineLen
		if cut > len(s) {
			cut = len(s)
		}
		result = append(result, s[:cut])
		s = s[cut:]
	}
	return strings.Join(result, "\n")
}

// indent adds a specified number of spaces to each line of a string.
func indent(s string, spaces int) string {
	padding := strings.Repeat(" ", spaces)
	return padding + strings.ReplaceAll(s, "\n", "\n"+padding)
}

// formatCritical returns "critical" if the bool is true, empty string otherwise.
func formatCritical(critical bool) string {
	if critical {
		return "critical"
	}
	return ""
}

// isExtensionCritical checks the raw extensions slice to find if a given OID is critical.
func isExtensionCritical(extensions []pkix.Extension, oid string) bool {
	for _, ext := range extensions {
		if ext.Id.String() == oid {
			return ext.Critical
		}
	}
	return false
}
