package renderjson

// `render-json` command takes the parsed TerragruntConfig struct and renders it out as JSON so that it can be processed by
// other tools. To make it easier to maintain, this uses the cty representation as an intermediary.
// NOTE: An unspecified advantage of using the cty representation is that the final block outputs would be a map
// representation, which is easier to work with than the list representation that will be returned by a naive go-struct
// to json conversion.
