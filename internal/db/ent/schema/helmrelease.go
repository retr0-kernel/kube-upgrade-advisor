package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// HelmRelease holds the schema definition for the HelmRelease entity.
type HelmRelease struct {
	ent.Schema
}

// Fields of the HelmRelease.
func (HelmRelease) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty(),
		field.String("namespace").
			NotEmpty(),
		field.String("chart").
			NotEmpty(),
		field.String("chart_version").
			NotEmpty(),
		field.String("app_version").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the HelmRelease.
func (HelmRelease) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cluster", Cluster.Type).
			Ref("helm_releases").
			Required().
			Unique(),
	}
}
