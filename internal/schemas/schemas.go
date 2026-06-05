package schemas

import _ "embed"

//go:embed bellows.schema.json
var Bellows []byte

//go:embed doctor.schema.json
var Doctor []byte

//go:embed explain.schema.json
var Explain []byte

//go:embed bundle.schema.json
var Bundle []byte
