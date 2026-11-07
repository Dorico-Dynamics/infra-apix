package spike

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The domain both engines serve. It holds no HTTP code, so the two engine
// implementations differ only in what an engine does: binding, validation,
// encoding, errors and the document.

var (
	errNotFound           = errors.New("not found")
	errPreconditionFailed = errors.New("the phase changed: If-Match does not match its current ETag")
	errVersionMismatch    = errors.New("expected_revision is not the document's current revision")
	errIdempotencyReuse   = errors.New("this Idempotency-Key was already used with a different request body")
	errInvalidCursor      = errors.New("the cursor is not one this API issued")
)

// Fixtures.
const (
	phaseID          = "7f8e6a2c-3b1d-4c5e-9f0a-1b2c3d4e5f60"
	payPeriodID      = 202610
	bankAccountID    = "3b2a1908-7f6e-4d5c-8b4a-392817160514"
	documentID       = "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
	brokenDocumentID = "5e4f3a2b-1c0d-4e9f-8a7b-6c5d4e3f2a1b" // its storage fails
	unknownID        = "00000000-0000-4000-8000-000000000000"
	clientID         = "spike-client"
	clientSecret     = "spike-secret"
)

// Limits.
const (
	statementMaxBytes = 1 << 20  // the upload cap, small so the 413 test stays cheap
	metadataMaxBytes  = 32 << 10 // the metadata body cap
	pageDefault       = 50
	pageMax           = 100
)

// phase is a project phase whose progress an action updates (route 1).
type phase struct {
	ID      string
	Percent float64
	Version int
}

func (p phase) etag() string { return `"` + strconv.Itoa(p.Version) + `"` }

// matches reports whether an If-Match value selects p: "*", or a list that
// holds p's ETag. Comparison is strong (RFC 9110, section 13.1.1).
func (p phase) matches(ifMatch string) bool {
	if strings.TrimSpace(ifMatch) == "*" {
		return true
	}
	for _, tag := range strings.Split(ifMatch, ",") {
		if strings.TrimSpace(tag) == p.etag() {
			return true
		}
	}
	return false
}

// payStub is an employee's pay stub for a pay period, served as a PDF
// (route 2).
type payStub struct {
	PayPeriodID int64
	Employee    string
}

// pdfBytes is a minimal PDF: enough for the content checks, not a layout.
func (p payStub) pdfBytes() []byte {
	return []byte("%PDF-1.7\n% pay stub " + p.Employee + " " + strconv.FormatInt(p.PayPeriodID, 10) + "\n%%EOF\n")
}

// importResult summarises a bank statement import (route 3).
type importResult struct {
	TotalProcessed int    `json:"total_processed"`
	Inserted       int64  `json:"inserted"`
	Duplicates     int64  `json:"duplicates"`
	BankAccountID  string `json:"bank_account_id"`
}

// documentMetadata is a document's metadata record, updated against the
// revision the caller read (route 4).
type documentMetadata struct {
	DocumentID string            `json:"document_id"`
	Revision   int64             `json:"revision"`
	Fields     map[string]string `json:"fields"`
}

// grant is listed with a cursor and created idempotently (route 5).
type grant struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Amount float64 `json:"amount"`
}

// tokenResponse is the RFC 6749 §5.1 success body (route 6).
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// oauthError is the RFC 6749 §5.2 error: one of a fixed set of codes, and an
// optional description.
type oauthError struct {
	Status      int    `json:"-"`
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (e *oauthError) Error() string { return e.Code + ": " + e.Description }

type store struct {
	mu          sync.Mutex
	phases      map[string]*phase
	payStubs    map[int64]payStub
	accounts    map[string]map[string]bool // bank account → statement lines seen
	metadata    map[string]*documentMetadata
	grants      []grant
	idempotency map[string]idempotentResult
	clients     map[string]string // client_id → secret
	nextID      int
}

// idempotentResult is a stored response to replay for a key.
type idempotentResult struct {
	Fingerprint string
	Status      int
	Grant       grant
}

func newStore() *store {
	s := &store{
		phases:      map[string]*phase{phaseID: {ID: phaseID, Percent: 10, Version: 1}},
		payStubs:    map[int64]payStub{payPeriodID: {PayPeriodID: payPeriodID, Employee: "E-1001"}},
		accounts:    map[string]map[string]bool{bankAccountID: {}},
		metadata:    map[string]*documentMetadata{documentID: {DocumentID: documentID, Revision: 1, Fields: map[string]string{"title": "Supply contract"}}},
		idempotency: map[string]idempotentResult{},
		clients:     map[string]string{clientID: clientSecret},
	}
	for i := 1; i <= 5; i++ {
		s.grants = append(s.grants, grant{ID: fmt.Sprintf("g-%03d", i), Title: fmt.Sprintf("Grant %d", i), Amount: float64(i) * 1000})
	}
	return s
}

func (s *store) updateProgress(id, ifMatch string, percent float64) (phase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.phases[id]
	if !ok {
		return phase{}, errNotFound
	}
	if !p.matches(ifMatch) {
		return phase{}, errPreconditionFailed
	}
	p.Percent = percent
	p.Version++
	return *p, nil
}

func (s *store) payStub(id int64) (payStub, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payStubs[id]
	if !ok {
		return payStub{}, errNotFound
	}
	return p, nil
}

// importStatement imports every line after the header; a line the account
// already holds is a duplicate.
func (s *store) importStatement(accountID string, data []byte) (importResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen, ok := s.accounts[accountID]
	if !ok {
		return importResult{}, errNotFound
	}
	res := importResult{BankAccountID: accountID}
	lines := bufio.NewScanner(bytes.NewReader(data))
	for header := true; lines.Scan(); header = false {
		line := strings.TrimSpace(lines.Text())
		if header || line == "" {
			continue
		}
		res.TotalProcessed++
		if seen[line] {
			res.Duplicates++
			continue
		}
		seen[line] = true
		res.Inserted++
	}
	return res, lines.Err()
}

func (s *store) updateMetadata(id string, expectedRevision int64, fields map[string]string) (documentMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == brokenDocumentID {
		return documentMetadata{}, fmt.Errorf("storage: write %s: dial tcp 192.0.2.7:5432: connection refused", id)
	}
	m, ok := s.metadata[id]
	if !ok {
		return documentMetadata{}, errNotFound
	}
	if expectedRevision != m.Revision {
		return documentMetadata{}, errVersionMismatch
	}
	m.Revision++
	m.Fields = maps.Clone(fields)
	return documentMetadata{DocumentID: m.DocumentID, Revision: m.Revision, Fields: maps.Clone(m.Fields)}, nil
}

// listGrants returns up to limit grants after the cursor, and the cursor of
// the next page ("" on the last page). A cursor is the opaque, base64url
// encoding of the last ID returned.
func (s *store) listGrants(cursor string, limit int) ([]grant, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := 0
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", errInvalidCursor
		}
		i := sort.Search(len(s.grants), func(i int) bool { return s.grants[i].ID > string(raw) })
		if i == 0 || s.grants[i-1].ID != string(raw) {
			return nil, "", errInvalidCursor
		}
		start = i
	}
	end := min(start+limit, len(s.grants))
	page := append([]grant(nil), s.grants[start:end]...)
	next := ""
	if end < len(s.grants) {
		next = base64.RawURLEncoding.EncodeToString([]byte(s.grants[end-1].ID))
	}
	return page, next, nil
}

// createGrant creates a grant, once per Idempotency-Key when one is given. A
// replay with the same body returns the stored result and replayed=true; the
// same key with another body is errIdempotencyReuse.
func (s *store) createGrant(key, title string, amount float64) (g grant, status int, replayed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum := sha256.Sum256([]byte(title + "\x00" + strconv.FormatFloat(amount, 'f', -1, 64)))
	fp := hex.EncodeToString(sum[:])
	if prev, ok := s.idempotency[key]; ok && key != "" {
		if prev.Fingerprint != fp {
			return grant{}, 0, false, errIdempotencyReuse
		}
		return prev.Grant, prev.Status, true, nil
	}
	s.nextID++
	g = grant{ID: fmt.Sprintf("g-new-%03d", s.nextID), Title: title, Amount: amount}
	s.grants = append(s.grants, g)
	if key != "" {
		s.idempotency[key] = idempotentResult{Fingerprint: fp, Status: 201, Grant: g}
	}
	return g, 201, false, nil
}

// token implements the client_credentials grant only.
func (s *store) token(grantType, id, secret, scope string) (tokenResponse, *oauthError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if grantType != "client_credentials" {
		return tokenResponse{}, &oauthError{Status: 400, Code: "unsupported_grant_type", Description: "only client_credentials is supported"}
	}
	if want, ok := s.clients[id]; !ok || want != secret {
		return tokenResponse{}, &oauthError{Status: 401, Code: "invalid_client", Description: "client authentication failed"}
	}
	return tokenResponse{AccessToken: "at-" + id, TokenType: "Bearer", ExpiresIn: 3600, Scope: scope}, nil
}
