package structure

type File struct {
	Version   uint      `json:"version"`
	Algorithm Algorithm `json:"algorithm"`
	KDF       KDFParams `json:"kdf"`
	Data      string    `json:"data"`
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
