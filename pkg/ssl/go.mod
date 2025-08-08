module github.com/amaumene/gostremiofr/pkg/ssl

go 1.24.3

require (
    github.com/amaumene/gostremiofr/pkg/httputil v0.0.0
    github.com/amaumene/gostremiofr/pkg/logger v0.0.0
)

replace (
    github.com/amaumene/gostremiofr/pkg/httputil => ../httputil
    github.com/amaumene/gostremiofr/pkg/logger => ../logger
)