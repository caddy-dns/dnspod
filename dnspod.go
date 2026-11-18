package dnspod

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/libdns/libdns"
	"github.com/nrdcg/dnspod-go"
)

// Record wraps libdns.RR to include the DNSPod record ID.
type Record struct {
	base libdns.RR
	ID   string
}

// RR returns the underlying libdns.RR struct.
func (r Record) RR() libdns.RR {
	return r.base
}

// Provider wraps the provider implementation as a Caddy module.
type Provider struct {
	APIToken string `json:"api_token,omitempty"`
}

func init() {
	caddy.RegisterModule(Provider{})
}

// CaddyModule returns the Caddy module information.
func (Provider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dns.providers.dnspod",
		New: func() caddy.Module { return new(Provider) },
	}
}

// Provision sets up the module.
func (p *Provider) Provision(ctx caddy.Context) error {
	p.APIToken = caddy.NewReplacer().ReplaceAll(p.APIToken, "")
	return nil
}

// UnmarshalCaddyfile sets up the DNS provider from Caddyfile tokens. Syntax:
//
// dnspod [<api_token>] {
//     api_token <api_token>
// }
func (p *Provider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			p.APIToken = d.Val()
		}
		if d.NextArg() {
			return d.ArgErr()
		}
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "api_token":
				if !d.NextArg() {
					return d.ArgErr()
				}
				if p.APIToken != "" {
					return d.Err("API token already set")
				}
				p.APIToken = d.Val()
				if d.NextArg() {
					return d.ArgErr()
				}
			default:
				return d.Errf("unrecognized subdirective '%s'", d.Val())
			}
		}
	}
	if p.APIToken == "" {
		return d.Err("missing API token")
	}
	return nil
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	client := p.getClient()
	zone = strings.Trim(zone, ".")
	domainID, err := p.getDomainID(zone)
	if err != nil {
		return nil, err
	}
	records, _, err := client.Records.List(domainID, "")
	if err != nil {
		return nil, err
	}

	var libdnsRecords []libdns.Record
	for _, record := range records {
		libdnsRecords = append(libdnsRecords, p.toLibdnsRecord(record))
	}
	return libdnsRecords, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	client := p.getClient()
	zone = strings.Trim(zone, ".")
	domainID, err := p.getDomainID(zone)
	if err != nil {
		return nil, err
	}
	var addedRecords []libdns.Record

	for _, libdnsRecord := range records {
		dnspodRecord := p.fromLibdnsRecord(libdnsRecord)
		created, _, err := client.Records.Create(domainID, dnspodRecord)
		if err != nil {
			return addedRecords, err
		}
		addedRecords = append(addedRecords, withID(libdnsRecord, created.ID))
	}

	return addedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing ones or creating new ones.
// It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	client := p.getClient()
	zone = strings.Trim(zone, ".")
	domainID, err := p.getDomainID(zone)
	if err != nil {
		return nil, err
	}
	var setRecords []libdns.Record

	for _, libdnsRecord := range records {
		id := ""
		if r, ok := libdnsRecord.(Record); ok {
			id = r.ID
		}
		dnspodRec := p.fromLibdnsRecord(libdnsRecord)
		if id == "" {
			created, _, err := client.Records.Create(domainID, dnspodRec)
			if err != nil {
				return setRecords, err
			}
			setRecords = append(setRecords, withID(libdnsRecord, created.ID))
			continue
		}

		// Set ID in the attributes for Update as DNSPod API often requires it in the body
		dnspodRec.ID = id
		_, _, err := client.Records.Update(domainID, id, dnspodRec)
		if err != nil {
			// Fallback: Delete and Re-create if Update fails (sometimes DNSPod API is picky about record IDs)
			_, _ = client.Records.Delete(domainID, id)
			created, _, err := client.Records.Create(domainID, dnspodRec)
			if err != nil {
				return setRecords, fmt.Errorf("update failed (%v) and fallback create also failed: %v", id, err)
			}
			setRecords = append(setRecords, withID(libdnsRecord, created.ID))
			continue
		}
		setRecords = append(setRecords, withID(libdnsRecord, id))
	}

	return setRecords, nil
}

// DeleteRecords deletes the records from the zone. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	client := p.getClient()
	zone = strings.Trim(zone, ".")
	domainID, err := p.getDomainID(zone)
	if err != nil {
		return nil, err
	}
	var deletedRecords []libdns.Record

	for _, libdnsRecord := range records {
		id := ""
		if r, ok := libdnsRecord.(Record); ok {
			id = r.ID
		}
		if id == "" {
			continue
		}
		_, err := client.Records.Delete(domainID, id)
		if err != nil {
			return deletedRecords, err
		}
		deletedRecords = append(deletedRecords, libdnsRecord)
	}

	return deletedRecords, nil
}

func (p *Provider) getClient() *dnspod.Client {
	return dnspod.NewClient(dnspod.CommonParams{
		LoginToken: p.APIToken,
	})
}

func (p *Provider) getDomainID(zone string) (string, error) {
	// If zone is already numeric, assume it's an ID
	if _, err := strconv.Atoi(zone); err == nil {
		return zone, nil
	}

	client := p.getClient()
	domains, _, err := client.Domains.List()
	if err != nil {
		return "", err
	}

	for _, d := range domains {
		if d.Name == zone {
			return d.ID.String(), nil
		}
	}

	return "", fmt.Errorf("domain %s not found", zone)
}

// withID returns rec with the DNSPod record ID attached. Record.Create and Record.Modify
// only echo back id, name and status, so a libdns record built from their responses
// would lose the type, value and TTL that callers (e.g. ACME DNS-01 solvers) rely on.
func withID(rec libdns.Record, id string) libdns.Record {
	return Record{ID: id, base: rec.RR()}
}

func (p *Provider) toLibdnsRecord(record dnspod.Record) libdns.Record {
	ttl, _ := strconv.Atoi(record.TTL)
	return Record{
		ID: record.ID,
		base: libdns.RR{
			Type: record.Type,
			Name: record.Name,
			Data: record.Value,
			TTL:  time.Duration(ttl) * time.Second,
		},
	}
}

func (p *Provider) fromLibdnsRecord(record libdns.Record) dnspod.Record {
	rr := record.RR()
	dnspodRec := dnspod.Record{
		Type:  rr.Type,
		Name:  rr.Name,
		Value: rr.Data,
		TTL:   fmt.Sprintf("%.0f", rr.TTL.Seconds()),
		Line:  "默认",
	}
	// Note: We don't set ID here by default as it's often used for Create/Update attributes
	// Note: dnspod-go Record.MX is used for priority.
	// Since libdns.RR doesn't expose it, we'll just handle basic types.
	return dnspodRec
}

// Interface guards
var (
	_ caddyfile.Unmarshaler = (*Provider)(nil)
	_ caddy.Provisioner     = (*Provider)(nil)
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
