module github.com/pesio-ai/be-ap-invoices

go 1.25.0

require (
	github.com/jackc/pgx/v5 v5.8.0
	github.com/pesio-ai/be-lib-common v0.0.0-20260124020211-8d03e3eed225
	github.com/pesio-ai/be-lib-proto v0.0.0-20260124164652-9c290ae7759a
	google.golang.org/grpc v1.78.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/pesio-ai/be-lib-common => ../be-lib-common

replace github.com/pesio-ai/be-lib-proto => ../be-lib-proto
