package blockchain

import (
	"blockchain_decentralization/wallet"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
)

func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer
	enc := gob.NewEncoder(&encoded)

	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

// Hashes the contents of the Transaction (Input and Output)
// Then sets it as an ID
func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encode := gob.NewEncoder(&encoded)
	err := encode.Encode(tx)
	Handle(err)

	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

// Records the first transaction on the blockchain
func CoinbaseTx(to, data string) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}

	txin := TxInput{[]byte{}, -1, nil, []byte(data)}
	txout := NewTXOutput(100, to)

	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}}
	tx.SetID()

	return &tx
}

func NewTransaction(from, to string, amount int, UTXO *UTXOSet) (*Transaction, error) {
	var inputs []TxInput
	var outputs []TxOutput

	wallets, err := wallet.CreateWallets()
	Handle(err)
	w := wallets.GetWallet(from)
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	acc, validOutputs := UTXO.FindSpendableOutputs(pubKeyHash, amount)

	if acc < amount {
		log.Panic("Error: not enough founds")
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			input := TxInput{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	outputs = append(outputs, *NewTXOutput(amount, to))

	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc - amount, from))
	}

	tx := Transaction{nil, inputs, outputs}
	tx.ID = tx.Hash()

	privKey, err := x509.ParseECPrivateKey(w.PrivateKey)
	
	if err == nil {		
		UTXO.Blockchain.SignTransaction(&tx, *privKey)
	}
	
	if err != nil {
		parsedKey, err := x509.ParsePKCS8PrivateKey(w.PrivateKey)
		Handle(err)

		ecdsaKey, ok := parsedKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("The key is not an ECDSA private key")
		}

		UTXO.Blockchain.SignTransaction(&tx, *ecdsaKey)
	}

	return &tx, nil
}

func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {
	if tx.IsCoinbase() {
		return
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction is not correct")
		}
	}

	txCopy := tx.TrimmedCopy()

	for inId, in := range txCopy.Inputs {
		prevTx := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inId].PubKey = nil

		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		Handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inId].Signature = signature
	}
}

func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	for _, in := range tx.Inputs {
		inputs = append(inputs, TxInput{in.ID, in.Out, nil, nil})
	}

	for _, out := range tx.Outputs {
		outputs = append(outputs, TxOutput{out.Value, out.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs}
	return txCopy
}

func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	for _, in := range tx.Inputs {
		if prevTXs[hex.EncodeToString(in.ID)].ID == nil {
			log.Panic("Previous transaction does not exist")
		}
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	for inId, in := range tx.Inputs {
		prevTx := prevTXs[hex.EncodeToString(in.ID)]
		txCopy.Inputs[inId].Signature = nil
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[in.Out].PubKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inId].PubKey = nil

		// 1. SIGNATURE PREPARATION
		// Modern Go prefers ASN.1 formatted signatures over raw big.Ints.
		sigLen := len(in.Signature)
		r := new(big.Int).SetBytes(in.Signature[:(sigLen / 2)])
		s := new(big.Int).SetBytes(in.Signature[(sigLen / 2):])

		// Define a temporary struct to quickly marshal the R and S values into ASN.1 DER
		type ecdsaSignature struct {
			R, S *big.Int
		}
		sigASN1, err := asn1.Marshal(ecdsaSignature{r, s})
		if err != nil {
			return false
		}

		// 2. PUBLIC KEY PREPARATION
		// Fix: Extract from in.PubKey, NOT in.Signature
		pubKeyBytes := in.PubKey
		
		// If the key is just 64 bytes (X and Y concatenated without prefix), 
		// we must prepend 0x04 to make it a valid SEC 1 uncompressed key.
		if len(pubKeyBytes) == 64 {
			pubKeyBytes = append([]byte{0x04}, pubKeyBytes...)
		}

		// Use the modern parser instead of manually constructing ecdsa.PublicKey
		pubKey, err := ecdsa.ParseUncompressedPublicKey(curve, pubKeyBytes)
		if err != nil {
			return false
		}

		// 3. VERIFICATION
		// Use VerifyASN1, which is the current, non-deprecated standard
		if !ecdsa.VerifyASN1(pubKey, txCopy.ID, sigASN1) {
			return false
		}
	}

	return true
}

func (tx Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("--- Transaction %x:", tx.ID))
	for i, input := range tx.Inputs {
		lines = append(lines, fmt.Sprintf("    Input     %d:", i))
		lines = append(lines, fmt.Sprintf("    TXID      %x:", input.ID))
		lines = append(lines, fmt.Sprintf("    Out       %d:", input.Out))
		lines = append(lines, fmt.Sprintf("    Signature %x:", input.Signature))
		lines = append(lines, fmt.Sprintf("    PubKey    %x:", input.PubKey))
	}

	for i, output := range tx.Outputs {
		lines = append(lines, fmt.Sprintf("    Output     %d:", i))
		lines = append(lines, fmt.Sprintf("    Value      %x:", output.Value))
		lines = append(lines, fmt.Sprintf("    Script     %d:", output.PubKeyHash))
	}

	return strings.Join(lines, "\n")
}
