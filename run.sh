APP_NAME="portal_workers"
go install .
go build -o $APP_NAME
./$APP_NAME -config=$1 -workers=$2