package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"log"
)

const (
	checksumLength = 4
	version        = byte(0x00)
)

type Wallet struct {
	PrivateKey []byte
	PublicKey  []byte
}

// Generates private key and public key
func NewKeyPair() ([]byte, []byte) {
	curve := elliptic.P256()

	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	private, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		log.Panic(err)
	}

	public, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		log.Fatalf("Failed to marshal public key: %v", err)
	}

	return private, public
}

func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	wallet := Wallet{private, public}

	return &wallet
}

func PublicKeyHash(pubKey []byte) []byte {
	pubHash := sha256.Sum256(pubKey)

	hash := sha256.Sum224(pubHash[:])
	return hash[:]
}

func Checksum(payload []byte) []byte {
	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLength]
}

func (w Wallet) Address() []byte {
	pubHash := PublicKeyHash(w.PublicKey)
	versionedHash := append([]byte{version}, pubHash...)

	checksum := Checksum(versionedHash)
	fullHash := append(versionedHash, checksum...)

	address := Base58Encode(fullHash)

	fmt.Printf("Public Key:  %x\n", w.PublicKey)
	fmt.Printf("Public Hash: %x\n", pubHash)
	fmt.Printf("Address:     %x\n", address)

	return address
}
