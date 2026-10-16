package verifier

type Verified struct {
	Subject string
	Roles   []string
	Scopes  []string
}

type Verifier interface {
	Verify(credential string) (*Verified, error)
}
