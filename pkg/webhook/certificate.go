// Copyright 2021 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	kerr "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rsaKeySize   = 2048
	duration365d = time.Hour * 24 * 365
	certName     = "multicluster-channel-webhook"
	tlsCrt       = "tls.crt"
	tlsKey       = "tls.key"
)

// Certificate defines a typical cert structure
type Certificate struct {
	Cert string
	Key  string
}

func getCASecretKey(whKey types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      fmt.Sprintf("%s-ca", whKey.Name),
		Namespace: whKey.Namespace,
	}
}

func getSignedCASecretKey(whKey types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      fmt.Sprintf("%s-signed-ca", whKey.Name),
		Namespace: whKey.Namespace,
	}
}

//getSelfSignedCACert will try to get the CA from the secret, if it doesn't exit, then
// it will generate a self singed CA cert and store it to the secret.
func getSelfSignedCACert(clt client.Client, certName string, whKey types.NamespacedName) (Certificate, error) {
	srtIns := &corev1.Secret{}
	ctx := context.TODO()

	persistSrtKey := getCASecretKey(whKey)
	//can't find the secret
	if err := clt.Get(ctx, persistSrtKey, srtIns); err != nil {
		if !kerr.IsNotFound(err) {
			return Certificate{}, fmt.Errorf("failed to get CA secret %w", err)
		}

		ca, err := GenerateSelfSignedCACert(certName)
		if err != nil {
			return ca, err
		}

		srtIns.Name = persistSrtKey.Name
		srtIns.Namespace = persistSrtKey.Namespace
		//this will be base64 encode on when viewing via kubectl
		srtIns.Type = corev1.SecretTypeTLS
		srtIns.Data = map[string][]byte{tlsCrt: []byte(ca.Cert), tlsKey: []byte(ca.Key)}

		if err := clt.Create(ctx, srtIns); err != nil {
			return Certificate{}, fmt.Errorf("failed to create CA secret %w", err)
		}

		return ca, nil
	}

	ca := Certificate{
		Cert: string(srtIns.Data[tlsCrt]),
		Key:  string(srtIns.Data[tlsKey]),
	}

	return ca, nil
}

func getSignedCert(clt client.Client, whKey types.NamespacedName,
	alternateDNS []string, ca Certificate) (Certificate, error) {
	srtIns := &corev1.Secret{}
	ctx := context.TODO()

	persistSrtKey := getSignedCASecretKey(whKey)

	//can't find the secret
	if err := clt.Get(ctx, persistSrtKey, srtIns); err != nil {
		if !kerr.IsNotFound(err) {
			return Certificate{}, fmt.Errorf("failed to get CA secret %w", err)
		}

		cert, err := GenerateSignedCert(whKey.Name, alternateDNS, ca)
		if err != nil {
			return cert, err
		}

		srtIns.Name = persistSrtKey.Name
		srtIns.Namespace = persistSrtKey.Namespace
		srtIns.Type = corev1.SecretTypeTLS
		srtIns.Data = map[string][]byte{tlsCrt: []byte(cert.Cert), tlsKey: []byte(cert.Key)}

		if err := clt.Create(ctx, srtIns); err != nil {
			return Certificate{}, fmt.Errorf("failed to create CA secret %w", err)
		}

		return cert, nil
	}

	cert := Certificate{
		Cert: string(srtIns.Data[tlsCrt]),
		Key:  string(srtIns.Data[tlsKey]),
	}

	return cert, nil
}

// GenerateWebhookCerts generate self singed CA and a signed cert pair. The
// signed pair is stored at the certDir. The CA will respect the inCluster DNS
func GenerateWebhookCerts(clt client.Client, certDir, webhookServiceNs, webhookServiceName string) ([]byte, error) {
	if len(certDir) == 0 {
		certDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")
	}

	whKey := types.NamespacedName{Name: webhookServiceName, Namespace: webhookServiceNs}

	ca, err := getSelfSignedCACert(clt, certName, whKey)
	if err != nil {
		return nil, err
	}

	alternateDNS := []string{
		fmt.Sprintf("%s.%s", webhookServiceName, webhookServiceNs),
		fmt.Sprintf("%s.%s.svc", webhookServiceName, webhookServiceNs),
		fmt.Sprintf("%s.%s.svc.cluster.local", webhookServiceName, webhookServiceNs),
	}

	cert, err := getSignedCert(clt, whKey, alternateDNS, ca)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(filepath.Join(certDir, tlsCrt), []byte(cert.Cert), os.FileMode(0600)); err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(filepath.Join(certDir, tlsKey), []byte(cert.Key), os.FileMode(0600)); err != nil {
		return nil, err
	}

	return []byte(ca.Cert), nil
}

// GenerateSelfSignedCACert generates a self signed CA
func GenerateSelfSignedCACert(cn string) (Certificate, error) {
	ca := Certificate{}

	template, err := generateBaseTemplateCert(cn, []string{})
	if err != nil {
		return ca, err
	}
	// Override KeyUsage and IsCA
	template.KeyUsage = x509.KeyUsageKeyEncipherment |
		x509.KeyUsageDigitalSignature |
		x509.KeyUsageCertSign
	template.IsCA = true

	priv, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return ca, fmt.Errorf("error generating rsa key: %s", err)
	}

	ca.Cert, ca.Key, err = getCertAndKey(template, priv, template, priv)

	return ca, err
}

// GenerateSignedCert generated cert pair which is signed by the self signed CA
func GenerateSignedCert(cn string, alternateDNS []string, ca Certificate) (Certificate, error) {
	cert := Certificate{}

	decodedSignerCert, _ := pem.Decode([]byte(ca.Cert))
	if decodedSignerCert == nil {
		return cert, errors.New("unable to decode certificate")
	}

	signerCert, err := x509.ParseCertificate(decodedSignerCert.Bytes)
	if err != nil {
		return cert, fmt.Errorf(
			"error parsing certificate: decodedSignerCert.Bytes: %s",
			err,
		)
	}

	decodedSignerKey, _ := pem.Decode([]byte(ca.Key))
	if decodedSignerKey == nil {
		return cert, errors.New("unable to decode key")
	}

	signerKey, err := x509.ParsePKCS1PrivateKey(decodedSignerKey.Bytes)
	if err != nil {
		return cert, fmt.Errorf(
			"error parsing prive key: decodedSignerKey.Bytes: %s",
			err,
		)
	}

	template, err := generateBaseTemplateCert(cn, alternateDNS)
	if err != nil {
		return cert, err
	}

	priv, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return cert, fmt.Errorf("error generating rsa key: %s", err)
	}

	cert.Cert, cert.Key, err = getCertAndKey(template, priv, signerCert, signerKey)

	return cert, err
}

func generateBaseTemplateCert(cn string, alternateDNS []string) (*x509.Certificate, error) {
	serialNumberUpperBound := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberUpperBound)

	if err != nil {
		return nil, err
	}

	return &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: cn,
		},
		IPAddresses: []net.IP{},
		DNSNames:    alternateDNS,
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(duration365d),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}, nil
}

func getCertAndKey(
	template *x509.Certificate,
	signeeKey *rsa.PrivateKey,
	parent *x509.Certificate,
	signingKey *rsa.PrivateKey,
) (string, string, error) {
	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		template,
		parent,
		&signeeKey.PublicKey,
		signingKey,
	)

	if err != nil {
		return "", "", fmt.Errorf("error creating certificate: %s", err)
	}

	certBuffer := bytes.Buffer{}
	if err := pem.Encode(
		&certBuffer,
		&pem.Block{Type: "CERTIFICATE", Bytes: derBytes},
	); err != nil {
		return "", "", fmt.Errorf("error pem-encoding certificate: %s", err)
	}

	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(
		&keyBuffer,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(signeeKey),
		},
	); err != nil {
		return "", "", fmt.Errorf("error pem-encoding key: %s", err)
	}

	return certBuffer.String(), keyBuffer.String(), nil
}
