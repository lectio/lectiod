Lectio GraphQL Schema Definitions
=================================
This package is called *schema_defn* instead of *schema* because the gqlgen package assumes neelance *schema* as
an alias in the code generates; so, schema_defn is used so as not to conflict. 

* models_concrete_generated.go is generated by gqlgen and should not be modified, use *make generate-graphql*
  target to regenerate (it's automatically regenerated by several *make* targets)
* resolvers_interface_generated.go is generated by gqlgen and should not be modified, use *make generate-graphql*
  target to regenerate (it's automatically regenerated by several other *make* targets)
* types.go is hand-written to implement the types in ../schema.types.json, all 'type' definitions in schema.graphql
  should be strongly-typed in go source files.