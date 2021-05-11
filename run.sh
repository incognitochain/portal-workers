APP_NAME="portal_workers"
go install .
go build -o $APP_NAME $1
./$APP_NAME