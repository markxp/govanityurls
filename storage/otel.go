package storage

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("github.com/markxp/govanityurls/storage")
