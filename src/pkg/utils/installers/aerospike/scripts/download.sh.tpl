# shellcheck disable=SC2148
# download the file
FILE_NAME="{{.FileName}}"
FILE_URL="{{.FileUrl}}"
fileDir=$(dirname "$FILE_NAME")
mkdir -p "$fileDir" || exit 1

curl -L -o "$FILE_NAME" "$FILE_URL" || exit 1

# check if the file exists
if [ ! -f "$FILE_NAME" ]; then
    echo "File $FILE_NAME not found after attempting to download"
    exit 1
fi
