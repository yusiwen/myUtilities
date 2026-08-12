package runner

import _ "embed"

// recipeSchema is the embedded copy of the recipe JSON Schema, kept in sync
// with docs/schema/recipe-schema.json. It is printed by `mu run schema` so
// users can configure editor validation (yaml-language-server $schema=...).
//
//go:embed recipe-schema.json
var recipeSchema []byte
