# TODO

## Tech Debt

### Migrate `q_dev` plugin from aws-sdk-go (v1) to aws-sdk-go-v2

`github.com/aws/aws-sdk-go` (v1) is deprecated and reached end of support on
July 31, 2025. `make lint` reports 11 `staticcheck SA1019` warnings for the
`q_dev` plugin. These are currently non-blocking in `scripts-local/build.sh`
(lint is a warning), but should be migrated to `aws-sdk-go-v2`.

Affected files:
- `backend/plugins/q_dev/tasks/identity_client.go`
- `backend/plugins/q_dev/tasks/s3_client.go`
- `backend/plugins/q_dev/tasks/s3_data_extractor.go`
- `backend/plugins/q_dev/tasks/s3_file_collector.go`

Required changes:
- Replace `github.com/aws/aws-sdk-go/aws` and `.../aws/credentials` with the
  v2 equivalents (`github.com/aws/aws-sdk-go-v2/aws`, `.../config`,
  `.../credentials`).
- Replace `.../aws/session` with v2 config loading
  (`config.LoadDefaultConfig`).
- Replace `.../service/identitystore` and `.../service/s3` with their v2
  service clients (`aws-sdk-go-v2/service/identitystore`, `.../service/s3`).
- Update `go.mod`/`go.sum` accordingly and remove the v1 dependency once unused.
- Re-run `make lint` to confirm the `SA1019` warnings are gone, then restore
  blocking lint in `scripts-local/build.sh`.

