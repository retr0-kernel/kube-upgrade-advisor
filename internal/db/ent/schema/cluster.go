package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Cluster holds the schema definition for the Cluster entity.
type Cluster struct {
	ent.Schema
}

// Fields of the Cluster.
func (Cluster) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name"),
		field.String("kube_version"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Cluster.
func (Cluster) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("helm_releases", HelmRelease.Type),
		edge.To("crds", CRD.Type),
		edge.To("manifest_apis", ManifestAPI.Type),
	}
}
