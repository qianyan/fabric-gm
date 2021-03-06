/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// +build ignore

//go:generate -command gencerts go run $GOPATH/src/github.com/tw-bc-group/fabric-gm/core/comm/testgmdata/certs/generate.go
//go:generate gencerts -orgs 2 -child-orgs 2 -servers 2 -clients 2

package main

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/Hyperledger-TWGC/tjfoc-gm/sm2"
	x509GM "github.com/Hyperledger-TWGC/tjfoc-gm/x509"
	"math/big"
	"net"
	"os"
	"time"
)

//command line flags
var (
	numOrgs        = flag.Int("orgs", 2, "number of unique organizations")
	numChildOrgs   = flag.Int("child-orgs", 2, "number of intermediaries per organization")
	numClientCerts = flag.Int("clients", 1, "number of client certificates per organization")
	numServerCerts = flag.Int("servers", 1, "number of server certificates per organization")
)

//default template for X509 subject
func subjectTemplate() pkix.Name {
	return pkix.Name{
		Country:  []string{"US"},
		Locality: []string{"San Francisco"},
		Province: []string{"California"},
	}
}

//default template for X509 certificates
func x509Template() (x509.Certificate, error) {

	//generate a serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return x509.Certificate{}, err
	}

	now := time.Now()
	//basic template to use
	x509 := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             now,
		NotAfter:              now.Add(3650 * 24 * time.Hour), //~ten years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	return x509, nil

}

//generate an EC private key (P256 curve)
func genKeyGM(name string) (*sm2.PrivateKey, error) {
	priv, err := sm2.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	//write key out to file
	keyBytes, err := x509GM.MarshalSm2UnecryptedPrivateKey(priv)
	keyFile, err := os.OpenFile(name+"-key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyFile.Close()
	return priv, nil
}

//generate a signed X509 certficate using ECDSA
func genCertificateGM(name string, template, parent *x509GM.Certificate, pub *sm2.PublicKey,
	priv *sm2.PrivateKey) (*x509GM.Certificate, error) {

	//create the sm2 public cert
	certBytes, err := x509GM.CreateCertificate(template, parent, pub, priv)
	if err != nil {
		return nil, err
	}

	//write cert out to file
	certFile, err := os.Create(name + "-cert.pem")
	if err != nil {
		return nil, err
	}
	//pem encode the cert
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	certFile.Close()

	x509Cert, err := x509GM.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	return x509Cert, nil
}

//generate an EC certificate appropriate for use by a TLS server
func genServerCertificateGM(name string, signKey *sm2.PrivateKey, signCert *x509GM.Certificate) error {
	fmt.Println(name)
	key, err := genKeyGM(name)
	template, err := x509Template()

	if err != nil {
		return err
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = "localhost"

	template.Subject = subject
	template.DNSNames = []string{"localhost"}
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}

	sm2Template := parseX509Certificate2Sm2(&template)

	_, err = genCertificateGM(name, sm2Template, signCert, &key.PublicKey, signKey)

	if err != nil {
		return err
	}

	return nil
}

//generate an EC certificate appropriate for use by a TLS server
func genClientCertificateGM(name string, signKey *sm2.PrivateKey, signCert *x509GM.Certificate) error {
	fmt.Println(name)
	key, err := genKeyGM(name)
	template, err := x509Template()

	if err != nil {
		return err
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	sm2Template := parseX509Certificate2Sm2(&template)
	_, err = genCertificateGM(name, sm2Template, signCert, &key.PublicKey, signKey)

	if err != nil {
		return err
	}

	return nil
}

func parseX509Certificate2Sm2(x509Cert *x509.Certificate) *x509GM.Certificate {
	sm2cert := &x509GM.Certificate{
		Raw:                     x509Cert.Raw,
		RawTBSCertificate:       x509Cert.RawTBSCertificate,
		RawSubjectPublicKeyInfo: x509Cert.RawSubjectPublicKeyInfo,
		RawSubject:              x509Cert.RawSubject,
		RawIssuer:               x509Cert.RawIssuer,

		Signature:          x509Cert.Signature,
		SignatureAlgorithm: x509GM.SM2WithSM3,

		PublicKeyAlgorithm: x509GM.PublicKeyAlgorithm(x509Cert.PublicKeyAlgorithm),
		PublicKey:          x509Cert.PublicKey,

		Version:      x509Cert.Version,
		SerialNumber: x509Cert.SerialNumber,
		Issuer:       x509Cert.Issuer,
		Subject:      x509Cert.Subject,
		NotBefore:    x509Cert.NotBefore,
		NotAfter:     x509Cert.NotAfter,
		KeyUsage:     x509GM.KeyUsage(x509Cert.KeyUsage),

		Extensions: x509Cert.Extensions,

		ExtraExtensions: x509Cert.ExtraExtensions,

		UnhandledCriticalExtensions: x509Cert.UnhandledCriticalExtensions,

		//ExtKeyUsage:	[]x509.ExtKeyUsage(x509Cert.ExtKeyUsage) ,
		UnknownExtKeyUsage: x509Cert.UnknownExtKeyUsage,

		BasicConstraintsValid: x509Cert.BasicConstraintsValid,
		IsCA:                  x509Cert.IsCA,
		MaxPathLen:            x509Cert.MaxPathLen,
		// MaxPathLenZero indicates that BasicConstraintsValid==true and
		// MaxPathLen==0 should be interpreted as an actual maximum path length
		// of zero. Otherwise, that combination is interpreted as MaxPathLen
		// not being set.
		MaxPathLenZero: x509Cert.MaxPathLenZero,

		SubjectKeyId:   x509Cert.SubjectKeyId,
		AuthorityKeyId: x509Cert.AuthorityKeyId,

		// RFC 5280, 4.2.2.1 (Authority Information Access)
		OCSPServer:            x509Cert.OCSPServer,
		IssuingCertificateURL: x509Cert.IssuingCertificateURL,

		// Subject Alternate Name values
		DNSNames:       x509Cert.DNSNames,
		EmailAddresses: x509Cert.EmailAddresses,
		IPAddresses:    x509Cert.IPAddresses,

		// Name constraints
		PermittedDNSDomainsCritical: x509Cert.PermittedDNSDomainsCritical,
		PermittedDNSDomains:         x509Cert.PermittedDNSDomains,

		// CRL Distribution Points
		CRLDistributionPoints: x509Cert.CRLDistributionPoints,

		PolicyIdentifiers: x509Cert.PolicyIdentifiers,
	}
	for _, val := range x509Cert.ExtKeyUsage {
		sm2cert.ExtKeyUsage = append(sm2cert.ExtKeyUsage, x509GM.ExtKeyUsage(val))
	}

	return sm2cert
}

//generate an EC certificate signing(CA) key pair and output as
//PEM-encoded files
func genCertificateAuthorityGM(name string) (*sm2.PrivateKey, *x509GM.Certificate, error) {

	key, err := genKeyGM(name)
	template, err := x509Template()

	if err != nil {
		return nil, nil, err
	}

	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = []byte{1, 2, 3, 4}

	sm2Tempate := parseX509Certificate2Sm2(&template)
	x509Cert, err := genCertificateGM(name, sm2Tempate, sm2Tempate, &key.PublicKey, key)

	if err != nil {
		return nil, nil, err
	}
	return key, x509Cert, nil
}

//generate an EC certificate appropriate for use by a TLS server
func genIntermediateCertificateAuthorityGM(name string, signKey *sm2.PrivateKey,
	signCert *x509GM.Certificate) (*sm2.PrivateKey, *x509GM.Certificate, error) {

	fmt.Println(name)
	key, err := genKeyGM(name)
	template, err := x509Template()

	if err != nil {
		return nil, nil, err
	}

	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}

	//set the organization for the subject
	subject := subjectTemplate()
	subject.Organization = []string{name}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = []byte{1, 2, 3, 4}

	sm2Template := parseX509Certificate2Sm2(&template)

	x509Cert, err := genCertificateGM(name, sm2Template, sm2Template, &key.PublicKey, signKey)

	if err != nil {
		return nil, nil, err
	}
	return key, x509Cert, nil
}

func main() {

	//parse the command line flags
	flag.Parse()

	fmt.Printf("Generating %d organizations each with %d server(s) and %d client(s)\n",
		*numOrgs, *numServerCerts, *numClientCerts)

	baseOrgName := "Org"
	//generate orgs / CAs
	for i := 1; i <= *numOrgs; i++ {
		signKey, signCert, err := genCertificateAuthorityGM(fmt.Sprintf(baseOrgName+"%d", i))
		if err != nil {
			fmt.Printf("error generating CA %s%d : %s\n", baseOrgName, i, err.Error())
		}
		//generate server certificates for the org
		for j := 1; j <= *numServerCerts; j++ {
			err := genServerCertificateGM(fmt.Sprintf(baseOrgName+"%d-server%d", i, j), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating server certificate for %s%d-server%d : %s\n",
					baseOrgName, i, j, err.Error())
			}
		}
		//generate client certificates for the org
		for k := 1; k <= *numClientCerts; k++ {
			err := genClientCertificateGM(fmt.Sprintf(baseOrgName+"%d-client%d", i, k), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating client certificate for %s%d-client%d : %s\n",
					baseOrgName, i, k, err.Error())
			}
		}
		//generate child orgs (intermediary authorities)
		for m := 1; m <= *numChildOrgs; m++ {
			childSignKey, childSignCert, err := genIntermediateCertificateAuthorityGM(
				fmt.Sprintf(baseOrgName+"%d-child%d", i, m), signKey, signCert)
			if err != nil {
				fmt.Printf("error generating CA %s%d-child%d : %s\n",
					baseOrgName, i, m, err.Error())
			}
			//generate server certificates for the child org
			for n := 1; n <= *numServerCerts; n++ {
				err := genServerCertificateGM(fmt.Sprintf(baseOrgName+"%d-child%d-server%d", i, m, n),
					childSignKey, childSignCert)
				if err != nil {
					fmt.Printf("error generating server certificate for %s%d-child%d-server%d : %s\n",
						baseOrgName, i, m, n, err.Error())
				}
			}
			//generate client certificates for the child org
			for p := 1; p <= *numClientCerts; p++ {
				err := genClientCertificateGM(fmt.Sprintf(baseOrgName+"%d-child%d-client%d", i, m, p),
					childSignKey, childSignCert)
				if err != nil {
					fmt.Printf("error generating server certificate for %s%d-child%d-client%d : %s\n",
						baseOrgName, i, m, p, err.Error())
				}
			}
		}
	}

}
