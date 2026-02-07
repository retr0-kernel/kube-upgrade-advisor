package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ManifestAPI holds the schema definition for the ManifestAPI entity.
type ManifestAPI struct {
	ent.Schema
}

// Fields of the ManifestAPI.
func (ManifestAPI) Fields() []ent.Field {
	return []ent.Field{
		field.String("group").
			Default(""), // Allow empty string for core APIs
		field.String("version").
			NotEmpty(),
		field.String("kind").
			NotEmpty(),
		field.Enum("source").
			Values("git", "local").
			Default("local"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the ManifestAPI.
func (ManifestAPI) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cluster", Cluster.Type).
			Ref("manifest_apis").
			Required().
			Unique(),
	}
}
