package garam

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// requestTimeout bounds one call to garam. It is what stops a listener that
// accepted the connection and then said nothing from holding a poll open until
// the manager stops.
const requestTimeout = 30 * time.Second

// Client reads this operator's definitions from garam's machine listener and
// claims them. What the caller is scopes both: garam answers the definitions
// naming the certificate's operator and no others.
type Client struct {
	address string
	http    *http.Client
}

// NewClient returns a Client reaching garam's machine listener at address, a
// host and port, over the given TLS configuration.
func NewClient(address string, tlsConfig *tls.Config) *Client {
	return &Client{
		address: address,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				// One connection per call. A reused connection performs no
				// handshake, and a handshake is where the certificate on disk
				// is read: kept alive between polls, this client would go on
				// presenting the certificate it started with.
				DisableKeepAlives: true,
			},
		},
	}
}

// ListDefinitions returns the definitions naming this operator, with the agent
// GRN each minted and whether it is already claimed.
func (c *Client) ListDefinitions(ctx context.Context) ([]Definition, error) {
	var answered []definitionPayload
	if err := c.send(ctx, http.MethodGet, "/definitions", &answered); err != nil {
		return nil, fmt.Errorf("list the definitions garam holds for this operator: %w", err)
	}
	definitions := make([]Definition, 0, len(answered))
	for _, payload := range answered {
		definition, err := payload.definition()
		if err != nil {
			return nil, fmt.Errorf("list the definitions garam holds for this operator: %w", err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// ClaimDefinition binds this operator to the agent a definition minted, which
// is what admits it to that agent's certificate route. An agent garam already
// holds a claim for answers [ErrClaimConflict], which no retry changes.
func (c *Client) ClaimDefinition(ctx context.Context, agent GRN) (Assignment, error) {
	var answered assignmentPayload
	err := c.send(ctx, http.MethodPost, "/definitions/"+url.PathEscape(string(agent))+"/claim", &answered)

	var refused *refusal
	if errors.As(err, &refused) && refused.status == http.StatusConflict {
		return Assignment{}, fmt.Errorf("claim %s: %w", agent, ErrClaimConflict)
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("claim %s: %w", agent, err)
	}
	return answered.assignment()
}

// send performs one request against the machine listener and decodes what it
// answered into out.
func (c *Client) send(ctx context.Context, method, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, method, "https://"+c.address+path, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return refusalFrom(response.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode what garam answered: %w", err)
	}
	return nil
}

// refusal is a status garam answered instead of the resource asked for, with
// the error kind and message it carries where it carries them.
type refusal struct {
	status  int
	kind    string
	message string
}

func (r *refusal) Error() string {
	if r.kind == "" {
		return fmt.Sprintf("garam answered %d", r.status)
	}
	return fmt.Sprintf("garam answered %d %s: %s", r.status, r.kind, r.message)
}

// refusalFrom reads garam's error body where the body is one, and reports the
// status alone where it is not: a refusal is a refusal whatever answered it.
func refusalFrom(status int, body []byte) *refusal {
	answered := struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(body, &answered); err != nil {
		return &refusal{status: status}
	}
	return &refusal{status: status, kind: answered.Kind, message: answered.Message}
}

// definitionPayload is one definition as the machine listener answers it.
type definitionPayload struct {
	AgentGRN string            `json:"agentGrn"`
	Values   map[string]string `json:"values"`
	Claim    *claimPayload     `json:"claim"`
}

type claimPayload struct {
	Epoch int64 `json:"epoch"`
}

func (p definitionPayload) definition() (Definition, error) {
	if p.AgentGRN == "" {
		return Definition{}, errors.New("a definition garam answered names no agent")
	}
	return Definition{
		Agent:   GRN(p.AgentGRN),
		Values:  p.Values,
		Claimed: p.Claim != nil,
	}, nil
}

// assignmentPayload is an assignment as the machine listener answers it.
type assignmentPayload struct {
	GRN      string `json:"grn"`
	Operator string `json:"operator"`
	Epoch    int64  `json:"epoch"`
}

func (p assignmentPayload) assignment() (Assignment, error) {
	if p.GRN == "" || p.Operator == "" {
		return Assignment{}, errors.New("an assignment garam answered names no agent or no operator")
	}
	return Assignment{Agent: GRN(p.GRN), Operator: GRN(p.Operator), Epoch: p.Epoch}, nil
}
