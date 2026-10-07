package hostproto

import (
	_ "embed" // the schemas are embedded as byte slices
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
)

// Identifiers of the host-protocol schemas. The request identifier is the URI the
// request format lives at; its major version is part of it.
const (
	// TicketType identifies the ticket schema.
	TicketType = "https://rachid-chabane.com/host-protocol/ticket/v1"
	// ResponseType identifies the response schema.
	ResponseType = "https://rachid-chabane.com/host-protocol/response/v1"
)

//go:embed schemas/request.json
var requestSchema []byte

//go:embed schemas/ticket.json
var ticketSchema []byte

//go:embed schemas/response.json
var responseSchema []byte

// Schemas returns the JSON Schema 2020-12 documents of the request, the ticket and
// the response, each identified by its `$id`. The response schema references the
// executor definition of the proof format's common schema, so a consumer loads
// both sets.
func Schemas() []format.Schema {
	return []format.Schema{
		{ID: RequestType, Bytes: requestSchema},
		{ID: TicketType, Bytes: ticketSchema},
		{ID: ResponseType, Bytes: responseSchema},
	}
}

// SchemaOf returns the schema with the identifier.
func SchemaOf(id string) (format.Schema, error) {
	for _, schema := range Schemas() {
		if schema.ID == id {
			return schema, nil
		}
	}
	return format.Schema{}, fmt.Errorf("no host-protocol schema %q", id)
}
