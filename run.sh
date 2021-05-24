APP_NAME="portal_workers"
go install .
go build -o $APP_NAME
while true
do
    ./$APP_NAME -config=$1
    echo "Crashed. Respawning..."
    sleep 1
done