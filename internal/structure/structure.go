package structure

type File struct {
	Algorithm Algorithm         `json:"algorithm"`
	KDF       KDFParams         `json:"kdf"`
	Verifier  string            `json:"verifier"`
	Records   []EncryptedRecord `json:"records"`
}

type EncryptedRecord struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type Algorithm struct {
	KDF    string `json:"kdf"`
	Cipher string `json:"cipher"`
}

type KDFParams struct {
	Salt        []byte `json:"salt"`
	Memory      uint32 `json:"memory"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
}
