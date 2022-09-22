if [ "$1" = "" ]
then
  echo "Usage: $0 file.go"
  exit 1
fi
egrep -o 'short:"[^ ]+' $1 |sort |uniq -c

