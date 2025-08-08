module github.com/amaumene/gostremiofr/pkg/torrentsearch

go 1.21

require github.com/cehbz/torrentname v1.2.1

replace (
	github.com/amaumene/gostremiofr/pkg/httputil => ../httputil
	github.com/amaumene/gostremiofr/pkg/logger => ../logger
	github.com/amaumene/gostremiofr/pkg/ratelimiter => ../ratelimiter
)
