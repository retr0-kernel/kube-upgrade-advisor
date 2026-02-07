package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// CRD holds the schema definition for the CRD entity.
type CRD struct {
	ent.Schema
}

// Fields of the CRD.
func (CRD) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty(),
		field.String("group").
			Default(""), // Allow empty for core APIs (though CRDs typically have groups)
		field.JSON("versions", []string{}).
			Optional(),
		field.String("helm_owner_name").
			Optional(),
		field.String("helm_owner_namespace").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the CRD.
func (CRD) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cluster", Cluster.Type).
			Ref("crds").
			Required().
			Unique(),
	}
}
