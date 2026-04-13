package server

import "encoding/json"

const (
	TypeRecfileIndex         = "recfile-index"
	TypeRecordTypeCollection = "record-type-collection"
	TypeRecord               = "record"
)

// Document is the top-level JSON:API response envelope.
type Document[A any] struct {
	Data     Resource[A]   `json:"data"`
	Included []AnyResource `json:"included,omitempty"`
	Links    Links         `json:"links,omitempty"`
}

// Resource is a typed JSON:API resource object.
type Resource[A any] struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id"`
	Attributes    A                       `json:"attributes,omitempty"`
	Relationships map[string]Relationship `json:"relationships,omitempty"`
	Links         Links                   `json:"links,omitempty"`
}

// AnyResource is used for included resources with runtime-variable attribute types.
type AnyResource struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id"`
	Attributes    json.RawMessage         `json:"attributes,omitempty"`
	Relationships map[string]Relationship `json:"relationships,omitempty"`
	Links         Links                   `json:"links,omitempty"`
}

// Relationship is a JSON:API relationship object. Data holds either a
// []RelationshipLinkage (to-many) or a RelationshipLinkage (to-one). JSON:API
// §7.5 requires to-one relationships to serialize as a single object and
// to-many relationships to serialize as an array; callers construct the
// appropriate shape via ToMany or ToOne.
type Relationship struct {
	Data  any   `json:"data"`
	Links Links `json:"links,omitempty"`
}

// ToMany builds a to-many relationship from a slice of linkages. An empty
// slice serializes as [] per JSON:API.
func ToMany(linkages []RelationshipLinkage) Relationship {
	if linkages == nil {
		linkages = []RelationshipLinkage{}
	}
	return Relationship{Data: linkages}
}

// ToOne builds a to-one relationship from a single linkage.
func ToOne(linkage RelationshipLinkage) Relationship {
	return Relationship{Data: linkage}
}

// RelationshipLinkage is a type/id pair in a relationship.
type RelationshipLinkage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Links is a map of link relation names to URLs.
type Links map[string]string
